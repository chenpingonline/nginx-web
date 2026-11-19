import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import { readFileSync } from 'node:fs';

const source = readFileSync(new URL('../../internal/httpapi/gateway_compat_bootstrap.js', import.meta.url), 'utf8');
function makeStorage() {
  const values = new Map();
  return { get length() { return values.size; }, key: i => [...values.keys()][i] ?? null, getItem: k => values.get(String(k)) ?? null, setItem: (k,v) => values.set(String(k),String(v)), removeItem:k=>values.delete(String(k)) };
}
function device(id='one', storage=makeStorage()) {
  const calls = [];
  class Element { setAttribute(k,v) { this[k] = v; } }
  class Resource extends Element { set href(v){this._href=v;} get href(){return this._href;} set src(v){this._src=v;} get src(){return this._src;} set action(v){this._action=v;} get action(){return this._action;} set poster(v){this._poster=v;} get poster(){return this._poster;} }
  class Document { get cookie(){return this.cookies || 'nas_session=private; ngw_other_sess_key=other';} set cookie(v){this.lastCookie=v;} addEventListener(){} }
  class XHR { open(...args){calls.push(['xhr',...args]);} }
  class Connection { constructor(...args){calls.push(['connection',...args]);} }
  const context = vm.createContext({URL, Request, Element, Document, XMLHttpRequest:XHR, WebSocket:Connection, EventSource:Connection,
    HTMLLinkElement:Resource, HTMLScriptElement:Resource, HTMLImageElement:Resource, HTMLFormElement:Resource, HTMLAnchorElement:Resource, HTMLIFrameElement:Resource, HTMLSourceElement:Resource, HTMLVideoElement:Resource,
    location:{host:'nas.example',protocol:'https:'},document:Object.assign(new Document(),{baseURI:'https://nas.example/app/nginx-web/proxy/'+id+'/pages/view.html'}),
    localStorage:storage,sessionStorage:makeStorage(), fetch(...args){calls.push(['fetch',...args]);return Promise.resolve();},open(...args){calls.push(['open',...args]);},history:{pushState(...args){calls.push(['push',...args]);},replaceState(...args){calls.push(['replace',...args]);}}
  });
  context.window=context;
  vm.runInContext(source.replace('__GATEWAY_PREFIX__',JSON.stringify('/app/nginx-web/proxy/'+id)).replace('__GATEWAY_COOKIE_PREFIX__',JSON.stringify('ngw_'+id+'_')).replace('__GATEWAY_TARGET__',JSON.stringify('http://192.168.1.2:8080/admin')),context);
  return {context,calls,run: code=>vm.runInContext(code,context)};
}

test('arbitrary API paths, target URLs, requests and streaming APIs stay in the selected service',async()=>{
 const d=device();
 await d.run(`fetch('/cgi/status'); fetch('http://192.168.1.2:8080/admin/api/state'); fetch('https://cdn.example/api/public'); fetch(new Request('https://nas.example/api/write',{method:'POST',body:'unchanged'})); new XMLHttpRequest().open('GET','/RPC2'); new WebSocket('ws://192.168.1.2:8080/admin/events'); new EventSource('/subscribe');`);
 assert.equal(d.calls[0][1],'https://nas.example/app/nginx-web/proxy/one/cgi/status');
 assert.equal(d.calls[1][1],'https://nas.example/app/nginx-web/proxy/one/api/state');
 assert.equal(d.calls[2][1],'https://cdn.example/api/public');
 const req=d.calls[3][1]; assert.equal(req.url,'https://nas.example/app/nginx-web/proxy/one/api/write');assert.equal(req.method,'POST');assert.equal(await req.text(),'unchanged');
 assert.equal(d.calls[4][2],'https://nas.example/app/nginx-web/proxy/one/RPC2');
 assert.equal(d.calls[5][1],'wss://nas.example/app/nginx-web/proxy/one/events');
 assert.equal(d.calls[6][1],'https://nas.example/app/nginx-web/proxy/one/subscribe');
});

test('Hash navigation, external resources and relative directories retain their meaning',()=>{
 const d=device();
 d.run(`history.replaceState(null,'','#/login'); history.pushState(null,'','/settings'); window.open('https://docs.example/help'); const image=new HTMLImageElement();image.src='../img/a.png';const link=new HTMLLinkElement();link.href='/assets/chunk.css';`);
 assert.equal(d.calls[0][3],'#/login');assert.equal(d.calls[1][3],'https://nas.example/app/nginx-web/proxy/one/settings');assert.equal(d.calls[2][1],'https://docs.example/help');
 assert.equal(d.run('image.src'),'https://nas.example/app/nginx-web/proxy/one/img/a.png');
 assert.equal(d.run('link.href'),'https://nas.example/app/nginx-web/proxy/one/assets/chunk.css');
});

test('device storage and cookies stay separate from NAS and other services',()=>{
 const storage=makeStorage();storage.setItem('username','NAS');
 const one=device('one',storage), two=device('two',storage);
 one.run(`localStorage.setItem('username','router');localStorage.token='a';document.cookie='sess_key=one; Path=/; Domain=nas.example; Secure';`);
 two.run(`localStorage.setItem('username','camera');localStorage.clear();`);
 assert.equal(storage.getItem('username'),'NAS');assert.equal(one.run(`localStorage.getItem('username')`),'router');assert.equal(two.run(`localStorage.getItem('username')`),null);
 assert.deepEqual(Array.from(one.run('Object.keys(localStorage)')).sort(),['token','username']);assert.equal(one.run('document.cookie'),'');
 assert.match(one.context.document.lastCookie,/^ngw_one_sess_key=one; Path=\/app\/nginx-web\/proxy\/one\//);assert.doesNotMatch(one.context.document.lastCookie,/Domain=/);
});
