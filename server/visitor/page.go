package visitor

import (
	"encoding/json"
	"strings"
)

type pageConfig struct {
	TunnelName string   `json:"tunnel_name"`
	SignalURL  string   `json:"signal_url"`
	ICEServers []string `json:"ice_servers"`
}

func renderPage(config pageConfig) ([]byte, error) {
	data, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	return []byte(strings.Replace(visitorPageTemplate, "__NRYNET_CONFIG__", string(data), 1)), nil
}

const visitorPageTemplate = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Nrynet Visitor P2P</title>
<style>
*{box-sizing:border-box}body{margin:0;background:#f4f7f5;color:#172235;font:14px system-ui,-apple-system,"Segoe UI",sans-serif}
main{max-width:820px;margin:48px auto;padding:0 20px}.panel{background:#fff;border:1px solid #dfe8e2;border-radius:10px;padding:24px;box-shadow:0 12px 30px #193b2410}
h1{font-size:22px;margin:0 0 8px}.muted{color:#6d7989}.status{display:inline-flex;gap:8px;align-items:center;margin:16px 0;padding:6px 10px;border-radius:999px;background:#edf2ef}.dot{width:8px;height:8px;border-radius:50%;background:#9ba7a0}.dot.ok{background:#11a45d}.dot.err{background:#d84c4c}
.form{display:grid;grid-template-columns:100px 1fr 100px;gap:10px;margin-top:12px}input,select,textarea,button{font:inherit;border:1px solid #d6e0da;border-radius:6px;padding:9px;background:#fff}input,textarea{min-width:0}textarea{width:100%;min-height:100px;margin-top:10px;resize:vertical}button{background:#14935a;color:#fff;border-color:#14935a;cursor:pointer}button:disabled{opacity:.55;cursor:not-allowed}.result{white-space:pre-wrap;overflow:auto;margin-top:14px;padding:14px;border-radius:6px;background:#14221b;color:#d9f5e4;min-height:160px}
@media(max-width:600px){main{margin:20px auto}.form{grid-template-columns:1fr 1fr}.form input{grid-column:1/-1}.form button{grid-column:1/-1}}
</style></head>
<body><main><section class="panel"><h1 id="title">Nrynet Visitor P2P</h1><div class="muted">浏览器与 Agent 直连，Server 仅负责信令。</div>
<div class="status"><i id="dot" class="dot"></i><span id="status">正在建立 P2P 连接...</span></div>
<div class="form"><input id="path" value="/" placeholder="路径"><select id="method"><option>GET</option><option>POST</option><option>PUT</option><option>DELETE</option></select><button id="send" disabled>发送请求</button></div>
<textarea id="body" placeholder="POST/PUT 请求体（可选）"></textarea><pre id="result" class="result">等待连接...</pre></section></main>
<script>
const config=__NRYNET_CONFIG__, pending=new Map();let sequence=0,channel;
const $=id=>document.getElementById(id), setStatus=(text,kind)=>{$("status").textContent=text;$('dot').className='dot '+(kind||'')};
const encode=value=>{const bytes=new TextEncoder().encode(value);let text='';for(const byte of bytes)text+=String.fromCharCode(byte);return btoa(text)};
const decode=value=>{const text=atob(value||'');const bytes=Uint8Array.from(text,char=>char.charCodeAt(0));return new TextDecoder().decode(bytes)};
function waitGathering(pc){if(pc.iceGatheringState==='complete')return Promise.resolve();return new Promise(resolve=>{pc.addEventListener('icegatheringstatechange',()=>pc.iceGatheringState==='complete'&&resolve(),{once:false})})}
function request(message){return new Promise((resolve,reject)=>{pending.set(message.id,{resolve,reject});channel.send(JSON.stringify(message))})}
async function connect(){
 try{
  const pc=new RTCPeerConnection({iceServers:(config.ice_servers||[]).map(urls=>({urls}))});channel=pc.createDataChannel('nrynet-visitor');
  channel.onopen=()=>{$('send').disabled=false;setStatus('P2P 已连接','ok')};channel.onclose=()=>{$('send').disabled=true;setStatus('P2P 已断开','err')};channel.onerror=()=>setStatus('P2P 通道错误','err');
  channel.onmessage=event=>{const message=JSON.parse(event.data),item=pending.get(message.id);if(!item)return;pending.delete(message.id);message.error?item.reject(new Error(message.error)):item.resolve(message)};
  const signal=new WebSocket(config.signal_url);const offer=await pc.createOffer();await pc.setLocalDescription(offer);await waitGathering(pc);
  await new Promise((resolve,reject)=>{signal.onopen=()=>{signal.send(JSON.stringify({kind:'offer',sdp:pc.localDescription.sdp}));resolve()};signal.onerror=reject});
  const answer=await new Promise((resolve,reject)=>{signal.onmessage=event=>{const value=JSON.parse(event.data);value.kind==='error'?reject(new Error(value.error)):resolve(value)};signal.onerror=reject});
  await pc.setRemoteDescription({type:'answer',sdp:answer.sdp});signal.close();
 }catch(error){setStatus(error.message||'P2P 连接失败','err');$('result').textContent='无法建立访客 P2P 连接：'+(error.message||error)}
}
async function send(){if(!channel||channel.readyState!=='open')return;const id=String(++sequence);$('result').textContent='请求中...';try{const response=await request({kind:'request',id,method:$('method').value,path:$('path').value||'/',headers:{'content-type':['text/plain']},body:encode($('body').value)});const headers=Object.entries(response.headers||{}).map(([key,value])=>key+': '+value.join(', ')).join('\n');$('result').textContent=response.status+'\n'+headers+'\n\n'+decode(response.body)}catch(error){$('result').textContent=error.message}}
$("send").onclick=send;$("title").textContent=config.tunnel_name+' · Visitor P2P';connect();
</script></body></html>`
