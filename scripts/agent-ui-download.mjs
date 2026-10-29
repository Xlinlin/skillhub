// Windows agent-browser canonicalizes download paths to a form Chrome rejects.
// Keep the same browser, set a normal path and perform a real CDP mouse click.
import { mkdir, rename, rmdir } from 'node:fs/promises';
import path from 'node:path';
import { randomUUID } from 'node:crypto';

const [endpoint, pageOrigin, selector, destination] = process.argv.slice(2);
if (!endpoint || !pageOrigin || !selector || !destination) throw new Error('Expected endpoint, origin, selector, destination');
const folder = path.join(path.dirname(path.resolve(destination)), `browser-download-${randomUUID()}`);
await mkdir(folder, { recursive: true });
const socket = new WebSocket(endpoint);
await new Promise((resolve, reject) => { socket.onopen = resolve; socket.onerror = reject; });
let sequence = 0;
const pending = new Map();
let suggestedName;
let finish;
let fail;
const completed = new Promise((resolve, reject) => { finish = resolve; fail = reject; });
const timeout = setTimeout(() => fail(new Error('Browser download timed out')), 30000);
socket.onmessage = event => {
  const message = JSON.parse(event.data);
  if (message.id && pending.has(message.id)) {
    const { resolve, reject } = pending.get(message.id); pending.delete(message.id);
    if (message.error) reject(new Error(JSON.stringify(message.error))); else resolve(message.result);
  }
  if (message.method === 'Browser.downloadWillBegin') suggestedName = message.params.suggestedFilename;
  if (message.method === 'Browser.downloadProgress') {
    if (message.params.state === 'completed') finish(message.params.filePath);
    if (message.params.state === 'canceled') fail(new Error('Chrome canceled the download'));
  }
};
const send = (method, params = {}, sessionId) => new Promise((resolve, reject) => {
  const id = ++sequence; pending.set(id, { resolve, reject });
  socket.send(JSON.stringify({ id, method, params, ...(sessionId ? { sessionId } : {}) }));
});
try {
  const { targetInfos } = await send('Target.getTargets');
  const page = targetInfos.find(target => target.type === 'page' && target.url.startsWith(pageOrigin));
  if (!page) throw new Error('Test browser page not found');
  await send('Browser.setDownloadBehavior', { behavior: 'allow', downloadPath: folder, eventsEnabled: true, browserContextId: page.browserContextId });
  const { sessionId } = await send('Target.attachToTarget', { targetId: page.targetId, flatten: true });
  const { result, exceptionDetails } = await send('Runtime.evaluate', { expression: `(() => { const a = document.querySelector(${JSON.stringify(selector)}); if (!a) throw new Error('Download control missing'); a.scrollIntoView({block:'center'}); const r = a.getBoundingClientRect(); return {x:r.x+r.width/2,y:r.y+r.height/2}; })()`, returnByValue: true }, sessionId);
  if (exceptionDetails) throw new Error('Cannot locate download control');
  await send('Input.dispatchMouseEvent', { type: 'mousePressed', ...result.value, button: 'left', clickCount: 1 }, sessionId);
  await send('Input.dispatchMouseEvent', { type: 'mouseReleased', ...result.value, button: 'left', clickCount: 1 }, sessionId);
  const savedPath = await completed;
  const source = savedPath || path.join(folder, suggestedName);
  if (path.relative(folder, source).startsWith('..')) throw new Error('Download escaped the test folder');
  await rename(source, path.resolve(destination));
  await rmdir(folder);
  console.log('Browser download saved');
} finally { clearTimeout(timeout); socket.close(); }
