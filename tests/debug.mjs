import assert from 'node:assert/strict';
import net from 'node:net';
const result = await new Promise((resolve, reject) => {
  const socket = net.connect(2345, '127.0.0.1');
  let data = '';
  socket.setTimeout(5000, () => socket.destroy(new Error('Debugger timeout')));
  socket.on('error', reject);
  socket.on('connect', () => socket.write(JSON.stringify({method:'RPCServer.GetVersion',params:[{}],id:1})+'\n'));
  socket.on('data', chunk => {
    data += chunk;
    if (data.includes('\n')) { socket.end(); resolve(JSON.parse(data.split('\n')[0])); }
  });
});
assert.equal(result.error, null);
assert.equal(result.result.APIVersion, 2);
assert.equal((await fetch('http://localhost:8080/api/health')).status, 200);
console.log('PASS: Delve JSON-RPC v2 and frontend/API health in debug mode');
