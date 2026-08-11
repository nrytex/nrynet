package visitor

import (
	"encoding/json"
	"strings"
)

type pageConfig struct {
	TunnelName string   `json:"tunnel_name"`
	SignalURL  string   `json:"signal_url"`
	ICEServers []string `json:"ice_servers"`
	ScopeURL   string   `json:"scope_url"`
	WorkerURL  string   `json:"worker_url"`
}

func renderPage(config pageConfig) ([]byte, error) {
	data, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	return []byte(strings.Replace(visitorPageTemplate, "__NRYNET_CONFIG__", string(data), 1)), nil
}

const visitorPageTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Nrynet Visitor</title>
<style>
html,body{height:100%;margin:0;background:#f4f7f5;color:#172235;font:14px system-ui,-apple-system,"Segoe UI",sans-serif}
#shell{height:100%;display:flex;flex-direction:column}
#bar{height:34px;display:flex;align-items:center;gap:10px;padding:0 14px;background:#14221b;color:#d9f5e4;flex:0 0 auto}
#dot{width:8px;height:8px;border-radius:50%;background:#e4ad39}#dot.ok{background:#27c77a}#dot.err{background:#e45a5a}
#status{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}#content{border:0;width:100%;flex:1;background:#fff}
</style></head>
<body><div id="shell"><div id="bar"><i id="dot"></i><span id="status">Connecting to the visitor tunnel...</span></div><iframe id="content" title="Visitor web application"></iframe></div>
<script>
const config=__NRYNET_CONFIG__,limits={maxPendingRequests:32,maxRequestBytes:16*1024*1024,maxBufferedBytes:4*1024*1024,lowBufferedBytes:1024*1024,sendTimeoutMs:30000},state={pending:new Map(),sendQueue:Promise.resolve(),sequence:0};let peer,channel;
const dot=document.getElementById('dot'),status=document.getElementById('status'),frame=document.getElementById('content');
function setStatus(value,kind){status.textContent=value;dot.className=kind||''}
function waitGathering(pc){if(pc.iceGatheringState==='complete')return Promise.resolve();return new Promise(resolve=>pc.addEventListener('icegatheringstatechange',()=>pc.iceGatheringState==='complete'&&resolve(),{once:false}))}
function wait(ms){return new Promise(resolve=>setTimeout(resolve,ms))}
async function waitSendCapacity(){const deadline=Date.now()+limits.sendTimeoutMs;if(!channel||channel.readyState!=='open')throw new Error('P2P channel is not open');while(channel.bufferedAmount>limits.maxBufferedBytes){if(Date.now()>=deadline)throw new Error('P2P send buffer stayed full');await wait(100)}}
function sendFrame(value){const task=state.sendQueue.catch(()=>{}).then(async()=>{if(!channel||channel.readyState!=='open')throw new Error('P2P channel is not open');await waitSendCapacity();channel.send(JSON.stringify(value))});state.sendQueue=task.catch(()=>{});return task}
function toBase64(bytes){let value='';for(let i=0;i<bytes.length;i+=0x8000)value+=String.fromCharCode(...bytes.subarray(i,i+0x8000));return btoa(value)}
function failPending(error){for(const [id,pending] of state.pending)pending.port.postMessage({kind:'response_end',id,error:error});state.pending.clear()}
function handleFrame(event){let message;try{message=JSON.parse(event.data)}catch{return}const pending=state.pending.get(message.id);if(!pending)return;pending.port.postMessage(message);if(message.kind==='response_end')state.pending.delete(message.id)}
function waitWorker(registration){if(registration.active)return Promise.resolve();return new Promise((resolve,reject)=>{const worker=registration.installing||registration.waiting;if(!worker){resolve();return}worker.addEventListener('statechange',()=>{if(worker.state==='activated')resolve();if(worker.state==='redundant')reject(new Error('visitor service worker was rejected'))})})}
async function forwardFetch(message,port){const body=message.body||new ArrayBuffer(0);if(state.pending.size>=limits.maxPendingRequests){port.postMessage({kind:'response_end',id:'',error:'too many visitor requests'});return}if(body.byteLength>limits.maxRequestBytes){port.postMessage({kind:'response_end',id:'',error:'visitor request body is too large'});return}const id=String(++state.sequence);state.pending.set(id,{port});try{await sendFrame({kind:'request_start',id,method:message.method,path:message.path,headers:message.headers||{}});const bytes=new Uint8Array(body);for(let offset=0;offset<bytes.length;offset+=24576)await sendFrame({kind:'request_chunk',id,body:toBase64(bytes.subarray(offset,Math.min(offset+24576,bytes.length)))});await sendFrame({kind:'request_end',id})}catch(error){state.pending.delete(id);port.postMessage({kind:'response_end',id,error:error.message||String(error)})}}
async function connect(){
 peer=new RTCPeerConnection({iceServers:(config.ice_servers||[]).map(urls=>({urls}))});channel=peer.createDataChannel('nrynet-visitor');channel.bufferedAmountLowThreshold=limits.lowBufferedBytes;channel.onmessage=handleFrame;
 channel.onopen=()=>{setStatus('P2P connected to Agent','ok');frame.src=config.scope_url};channel.onclose=()=>{failPending('P2P connection closed');setStatus('P2P connection closed','err');frame.removeAttribute('src')};channel.onerror=()=>{failPending('P2P data channel error');setStatus('P2P data channel error','err')};
 const signal=new WebSocket(config.signal_url),answer=new Promise((resolve,reject)=>{signal.onmessage=event=>{const value=JSON.parse(event.data);value.kind==='error'?reject(new Error(value.error)):value.kind==='answer'&&resolve(value)};signal.onerror=reject});
 await new Promise((resolve,reject)=>{signal.onopen=()=>{peer.createOffer().then(offer=>peer.setLocalDescription(offer)).then(()=>waitGathering(peer)).then(()=>{signal.send(JSON.stringify({kind:'offer',sdp:peer.localDescription.sdp}));resolve()}).catch(reject);signal.onerror=reject}});
 const value=await answer;await peer.setRemoteDescription({type:'answer',sdp:value.sdp});signal.close()
}
navigator.serviceWorker.addEventListener('message',event=>{if(event.data&&event.data.kind==='visitor_fetch'&&event.ports[0])forwardFetch(event.data,event.ports[0])});
(async()=>{try{const registration=await navigator.serviceWorker.register(config.worker_url,{scope:config.scope_url});await waitWorker(registration);await connect()}catch(error){setStatus(error.message||'Unable to establish visitor P2P','err')}})();
</script></body></html>`
