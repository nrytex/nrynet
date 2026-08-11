package visitor

const visitorServiceWorker = `
const scopePath=new URL(self.registration.scope).pathname;
const shellPath=scopePath.slice(0,-1);
const maxProxyResponseBytes=64*1024*1024;
self.addEventListener('install',event=>event.waitUntil(self.skipWaiting()));
self.addEventListener('activate',event=>event.waitUntil(self.clients.claim()));
self.addEventListener('fetch',event=>{
 const url=new URL(event.request.url);
 if(url.origin!==self.location.origin||!url.pathname.startsWith(scopePath)||url.pathname===scopePath+'sw.js')return;
 event.respondWith(proxy(event.request));
});
async function proxy(request){
 const windows=await self.clients.matchAll({type:'window',includeUncontrolled:true});
 const broker=windows.find(client=>new URL(client.url).pathname===shellPath)||windows.find(client=>new URL(client.url).pathname.startsWith(shellPath));
 if(!broker)return new Response('visitor broker is unavailable',{status:502});
 const body=await request.arrayBuffer(),headers={};
 request.headers.forEach((value,name)=>{if(name!=='host'&&name!=='content-length')(headers[name]||(headers[name]=[])).push(value)});
 const url=new URL(request.url),path='/'+url.pathname.slice(scopePath.length)+url.search;
 const port=new MessageChannel(),result=await responseStream(broker,port,{
  kind:'visitor_fetch',method:request.method,path:path==='/'?'/' : path,headers:headers,body:body
 });
 return result;
}
function responseStream(broker,channel,message){
 return new Promise(resolve=>{
  let controller,started=false,total=0,info=null,parts=[];
  let startResolve,startReject;
  const startedPromise=new Promise((resolveStart,rejectStart)=>{startResolve=resolveStart;startReject=rejectStart});
  const stream=new ReadableStream({start(value){controller=value}});
  channel.port1.onmessage=event=>{
   const data=event.data||{};
   if(data.kind==='response_start'){info=data;started=true;startResolve(data);return}
   if(data.kind==='response_chunk'){
    const bytes=fromBase64(data.body||'');total+=bytes.byteLength;
    if(total>maxProxyResponseBytes){controller.error(new Error('visitor response is too large'));channel.port1.close();return}
    if(isRewriteable(info))parts.push(bytes);else controller.enqueue(bytes);return
   }
   if(data.kind==='response_end'){
    channel.port1.close();
    if(data.error){if(!started)startReject(new Error(data.error));else controller.error(new Error(data.error));return}
    if(!started){startReject(new Error('visitor response did not start'));return}
    if(isRewriteable(info))controller.enqueue(rewriteBody(concat(parts),info.headers||{}));
    controller.close();
   }
  };
  channel.port1.start();
  broker.postMessage(message,[channel.port2,message.body]);
  startedPromise.then(start=>{const headers=toHeaders(start.headers||{});resolve(new Response(stream,{status:validStatus(start.status),headers:headers}))}).catch(error=>resolve(new Response(error.message,{status:502})))
 });
}
function validStatus(value){return value>=200&&value<=599?value:502}
function toHeaders(values){const headers=new Headers();Object.entries(values||{}).forEach(([name,list])=>(list||[]).forEach(value=>headers.append(name,value)));return headers}
function isRewriteable(info){const values=info&&info.headers||{};let content='';Object.entries(values).forEach(([name,list])=>{if(name.toLowerCase()==='content-type')content=(list||[]).join(';')});return /text\/html|text\/css/i.test(content)}
function concat(parts){const size=parts.reduce((total,value)=>total+value.byteLength,0),result=new Uint8Array(size);let offset=0;parts.forEach(value=>{result.set(value,offset);offset+=value.byteLength});return result}
function fromBase64(value){const text=atob(value),result=new Uint8Array(text.length);for(let i=0;i<text.length;i++)result[i]=text.charCodeAt(i);return result}
function toBase64(value){let text='';for(let i=0;i<value.length;i+=0x8000)text+=String.fromCharCode(...value.subarray(i,i+0x8000));return btoa(text)}
function rewriteBody(bytes,headers){const type=Object.entries(headers).find(([name])=>name.toLowerCase()==='content-type');const content=type?type[1].join(';'):'';const text=new TextDecoder().decode(bytes),prefix=shellPath,rewritten=/text\/html/i.test(content)?rewriteHTML(text,prefix):text.replace(/url\(\s*\/(?!\/)/gi,'url('+prefix+'/');return new TextEncoder().encode(rewritten)}
function rewriteHTML(text,prefix){const root=/\b(?:src|href|action|poster)=(["'])\/(?!\/)/gi;let value=text.replace(root,(match,quote)=>match.slice(0,-1)+prefix+'/');value=value.replace(/<head\b[^>]*>/i,match=>match+'<base href="'+prefix+'/">'+bridgeScript(prefix));return value}
function bridgeScript(prefix){const value=JSON.stringify(prefix);return '<script>(function(){var p='+value+',r=function(v){if(typeof v!=="string")return v;try{var u=new URL(v,location.href);if(u.origin===location.origin&&u.pathname.charAt(0)==="/"&&u.pathname.indexOf(p)!==0)return p+u.pathname+u.search+u.hash}catch(e){}return v},f=window.fetch;window.fetch=function(i,o){if(i instanceof Request)return f(new Request(r(i.url),i),o);return f(r(i),o)};var open=XMLHttpRequest.prototype.open;XMLHttpRequest.prototype.open=function(){if(arguments.length>1)arguments[1]=r(arguments[1]);return open.apply(this,arguments)}})();</script>'}
`
