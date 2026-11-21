package service

import (
	"errors"
	"strings"

	"github.com/chenpingonline/nginx-web/internal/domain"
)

func (s *AppService) SaveCustomConfig(original string, input domain.CustomConfig) (domain.CustomConfig, error) {
	return s.UpdateCustomConfig(original, input, nil)
}

func (s *AppService) UpdateCustomConfig(original string, input domain.CustomConfig, expected *string) (domain.CustomConfig, error) {
	input.Name = strings.TrimSpace(input.Name)
	if !strings.HasSuffix(input.Content, "\n") {
		input.Content += "\n"
	}
	err := s.updateState(func(state *State) error {
		configs := append([]domain.CustomConfig(nil), state.CustomConfigs...)
		found := false
		if original != "" {
			for index := range configs {
				if configs[index].Name == original {
					if expected != nil && configs[index].Content != *expected {
						return errors.New("源文件已更改，请刷新列表后重新编辑")
					}
					configs[index] = input
					found = true
					break
				}
			}
			if !found {
				return errors.New("找不到指定自定义配置文件")
			}
		} else {
			configs = append(configs, input)
		}
		if err := domain.ValidateCustomConfigs(configs); err != nil {
			return err
		}
		state.CustomConfigs = configs
		state.Dirty = true
		return nil
	})
	return input, err
}

func (s *AppService) DeleteCustomConfig(name string) error {
	return s.updateState(func(state *State) error {
		for index := range state.CustomConfigs {
			if state.CustomConfigs[index].Name == name {
				state.CustomConfigs = append(state.CustomConfigs[:index], state.CustomConfigs[index+1:]...)
				state.Dirty = true
				return nil
			}
		}
		return errors.New("找不到指定自定义配置文件")
	})
}
