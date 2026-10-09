package acme

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-acme/lego/v5/challenge"
	"github.com/go-acme/lego/v5/challenge/dns01"
)

type workerMessage struct {
	Stage      string      `json:"stage,omitempty"`
	Record     *Record     `json:"record,omitempty"`
	Checkpoint bool        `json:"checkpoint,omitempty"`
	Done       bool        `json:"done,omitempty"`
	OK         bool        `json:"ok,omitempty"`
	Error      *Diagnostic `json:"error,omitempty"`
	Log        *LogEntry   `json:"log,omitempty"`
}

// Every native factory runs in a separate process because upstream reads process
// environment variables. Credentials never enter command arguments or logs.
func workerEnvironment(in Input) []string {
	blocked := map[string]bool{}
	for _, p := range catalog.Providers {
		for _, f := range p.Fields {
			blocked[f.Key] = true
			blocked[f.Key+"_FILE"] = true
		}
	}
	env := []string{}
	for _, pair := range os.Environ() {
		k, _, _ := strings.Cut(pair, "=")
		if !blocked[k] {
			env = append(env, pair)
		}
	}
	values := map[string]string{}
	for k, v := range in.DNSConfig {
		if v != "" {
			values[k] = v
		}
	}
	// Apply the shared DNS wait setting unless a provider-specific override is set.
	if p := providerDefinition(in.Provider); p != nil {
		for _, f := range p.Fields {
			if strings.HasSuffix(f.Key, "_PROPAGATION_TIMEOUT") && values[f.Key] == "" {
				values[f.Key] = strconv.Itoa(in.PropagationSeconds)
			}
		}
	}
	for k, v := range values {
		env = append(env, k+"="+v)
	}
	return env
}

func issueInWorker(ctx context.Context, r *Record, checkpoint func() error, progress func(string)) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	out, writer, err := os.Pipe()
	if err != nil {
		return err
	}
	defer out.Close()
	defer writer.Close()
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(childCtx, executable, "acme-worker")
	cmd.Env = workerEnvironment(r.Input)
	cmd.ExtraFiles = []*os.File{writer}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	defer stdin.Close()
	if err = cmd.Start(); err != nil {
		return err
	}
	defer func() { cancel(); _ = cmd.Wait() }()
	_ = writer.Close()
	input := json.NewEncoder(stdin)
	if err = input.Encode(r); err != nil {
		return err
	}
	output := json.NewDecoder(out)
	for {
		var message workerMessage
		if err = output.Decode(&message); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return diagnostic("worker")
		}
		if message.Stage != "" {
			progress(message.Stage)
		}
		if message.Log != nil {
			emitLog(ctx, *message.Log)
		}
		if message.Record != nil {
			// Preserve the parent's live history and public job. Only issuer
			// checkpoints (never configuration or logs) travel back from the child.
			r.AccountKey, r.Registration = message.Record.AccountKey, message.Record.Registration
			r.Resource, r.Pending, r.RenewAt = message.Record.Resource, message.Record.Pending, message.Record.RenewAt
		}
		if message.Checkpoint {
			if err = checkpoint(); err != nil {
				return err
			}
			if err = input.Encode(true); err != nil {
				return err
			}
		}
		if message.Done {
			if !message.OK {
				if message.Error != nil {
					return classifyError(message.Error, message.Error.Stage)
				}
				return diagnostic("issuance")
			}
			return nil
		}
	}
}

// RunWorker is an internal executable mode. A dedicated pipe (fd 3) keeps
// third-party stdout/stderr separate from private checkpoint messages.
func RunWorker() int { return runWorker(nil) }

func runWorker(options []dns01.ChallengeOption) int {
	out := os.NewFile(3, "acme-checkpoints")
	if out == nil {
		return 1
	}
	defer out.Close()
	input := json.NewDecoder(io.LimitReader(os.Stdin, 16<<20))
	var r Record
	if err := input.Decode(&r); err != nil {
		return 1
	}
	if err := validate(&r.Input); err != nil {
		return 1
	}
	output := json.NewEncoder(out)
	var outputMu sync.Mutex
	send := func(message workerMessage) error {
		outputMu.Lock()
		defer outputMu.Unlock()
		return output.Encode(message)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Minute)
	defer cancel()
	ctx = withLogSink(ctx, func(entry LogEntry) { _ = send(workerMessage{Log: &entry}) })
	if len(r.Input.DNSResolvers) > 0 {
		configureDNSClient(r.Input)
	}
	for _, resolver := range resolverAddresses(r.Input) {
		dnsLog(ctx, "", "", resolver, 0, time.Now(), "dns_resolver")
	}
	if r.Input.DNSCheckMode == "wait" {
		options = append(options, dns01.WrapPreCheck(fixedWaitCheck(time.Duration(r.Input.DNSWaitSeconds)*time.Second)))
	}
	checkpoint := func() error {
		if err := send(workerMessage{Record: &r, Checkpoint: true}); err != nil {
			return err
		}
		var ok bool
		if err := input.Decode(&ok); err != nil {
			return err
		}
		if !ok {
			return errors.New("checkpoint failed")
		}
		return nil
	}
	stage := "准备申请"
	var stageMu sync.Mutex
	progress := func(message string) {
		stageMu.Lock()
		defer stageMu.Unlock()
		message = safeStage(message)
		if !cleanupStage(message) {
			stage = message
		}
		_ = send(workerMessage{Stage: message})
	}
	factory := func(in Input) (challenge.Provider, error) {
		progress("初始化 DNS 服务商")
		return newNativeDNSProvider(in.Provider)
	}
	err := issueWithDNS(ctx, &r, checkpoint, progress, factory, options)
	var detail *Diagnostic
	if err != nil {
		stageMu.Lock()
		detail = classifyError(err, stage)
		stageMu.Unlock()
	}
	if send(workerMessage{Record: &r, Done: true, OK: err == nil, Error: detail}) != nil {
		return 1
	}
	if err != nil {
		return 1
	}
	return 0
}
