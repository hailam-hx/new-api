/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
// These are manual recipes, independent of generated buffered HTTP methods.
// Fixtures prove client lifecycle handling; backend protocol tests remain separate.
export const nodeStreamingRecipe = String.raw`import assert from 'node:assert/strict';
import { WebSocket, WebSocketServer } from 'ws';
import { spawn } from 'node:child_process';

async function* sseEvents(body) {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let pending = '', data = [], event = '', id = '';
  try {
    while (true) {
      const chunk = await reader.read();
      pending += chunk.done ? decoder.decode() : decoder.decode(chunk.value, { stream: true });
      let end;
      while ((end = pending.indexOf('\n')) >= 0) {
        const line = pending.slice(0, end).replace(/\r$/, ''); pending = pending.slice(end + 1);
        if (line === '') {
          if (data.length) yield { event, id, data: data.join('\n') };
          data = []; event = '';
        } else if (!line.startsWith(':')) {
          const colon = line.indexOf(':');
          const field = colon < 0 ? line : line.slice(0, colon);
          const value = colon < 0 ? '' : line.slice(colon + 1).replace(/^ /, '');
          if (field === 'data') data.push(value);
          if (field === 'event') event = value;
          if (field === 'id' && !value.includes('\0')) id = value;
        }
      }
      if (chunk.done) break; // Do not dispatch an unterminated event at EOF.
    }
  } finally { await reader.cancel(); reader.releaseLock(); }
}
const wire = ': heartbeat\r\nid: one\r\nevent: response.output_text.delta\r\ndata: {"delta":"xin chào"}\r\n\r\ndata: first\ndata: second\n\ndata: [DONE]\n\n';
const bytes = new TextEncoder().encode(wire);
let position = 0, cancelled = false;
const body = new ReadableStream({ pull(controller) {
  if (position === bytes.length) return controller.close();
  controller.enqueue(bytes.slice(position, ++position));
}, cancel() { cancelled = true; } });
const events = [];
for await (const frame of sseEvents(body)) events.push(frame);
assert.deepEqual(events, [
  { event: 'response.output_text.delta', id: 'one', data: '{"delta":"xin chào"}' },
  { event: '', id: 'one', data: 'first\nsecond' },
  { event: '', id: 'one', data: '[DONE]' },
]);
assert.equal(JSON.parse(events[0].data).delta, 'xin chào');
let cancelCount = 0;
const active = new ReadableStream({ start(controller) { controller.enqueue(new TextEncoder().encode('data: first\n\n')); }, cancel() { cancelCount++; } });
for await (const frame of sseEvents(active)) { assert.equal(frame.data, 'first'); break; }
assert.equal(cancelCount, 1);
const truncated = new ReadableStream({ start(controller) { controller.enqueue(new TextEncoder().encode('data: unfinished')); controller.close(); } });
const incomplete = []; for await (const frame of sseEvents(truncated)) incomplete.push(frame);
assert.deepEqual(incomplete, []);

// Loopback-only transport; fixture credential has no provider/API authority.
const server = new WebSocketServer({ host: '127.0.0.1', port: 0, path: '/v1/responses' });
await new Promise(resolve => server.once('listening', resolve));
const port = server.address().port;
let opened = 0, closed = 0, submitted = 0, cancellations = 0;
server.on('connection', (socket, request) => {
  assert.equal(request.headers.authorization, 'Bearer fixture-only-token'); opened++;
  socket.on('close', () => closed++);
  socket.on('message', raw => {
    const message = JSON.parse(raw.toString());
    if (message.type === 'response.create') {
      assert.equal(message.model, 'fixture-model'); submitted++;
      socket.send(JSON.stringify({ type: 'response.created', response: { id: 'fixture-response' } }));
      socket.send(JSON.stringify({ type: 'response.output_text.delta', delta: 'fixture' }));
    } else if (message.type === 'response.cancel') {
      cancellations++;
      socket.send(JSON.stringify({ type: 'response.cancelled', response: { id: 'fixture-response' } }));
    } else socket.send(JSON.stringify({ type: 'error', status: 400, error: { message: 'Unsupported event' } }));
  });
});
try {
  const socket = new WebSocket('ws://127.0.0.1:' + port + '/v1/responses', { headers: { Authorization: 'Bearer fixture-only-token' } });
  const received = [];
  const completion = new Promise((resolve, reject) => {
    socket.on('error', reject);
    socket.on('message', raw => {
      const message = JSON.parse(raw.toString()); received.push(message.type);
      if (message.type === 'response.output_text.delta') socket.send(JSON.stringify({ type: 'response.cancel' }));
      if (message.type === 'response.cancelled') socket.close(1000);
    });
    socket.on('close', (code) => { try { assert.equal(code, 1000); resolve(); } catch (error) { reject(error); } });
  });
  socket.on('open', () => socket.send(JSON.stringify({ type: 'response.create', model: 'fixture-model', input: 'Hello' })));
  await completion;
  assert.deepEqual(received, ['response.created', 'response.output_text.delta', 'response.cancelled']);
  await new Promise((resolve, reject) => {
    const child = spawn(process.argv[2], ['streaming.py', String(port)], { stdio: 'inherit' });
    child.on('error', reject); child.on('close', code => code === 0 ? resolve() : reject(new Error('Python streaming fixture failed: ' + code)));
  });
} finally {
  for (const socket of server.clients) socket.terminate();
  await new Promise(resolve => server.close(resolve));
}
assert.equal(opened, 2); assert.equal(closed, 2); assert.equal(submitted, 2); assert.equal(cancellations, 2);
console.log('TypeScript manual SSE incremental UTF-8/framing/cancellation and WebSocket create/cancel/close fixtures PASS');
`

export const pythonStreamingRecipe = String.raw`import codecs
import json
import sys
from websockets.sync.client import connect

def sse_events(chunks):
    decoder = codecs.getincrementaldecoder('utf-8')()
    pending, data, event, event_id = '', [], '', ''
    try:
        for chunk in chunks:
            pending += decoder.decode(chunk)
            while '\n' in pending:
                line, pending = pending.split('\n', 1)
                line = line.removesuffix('\r')
                if not line:
                    if data: yield {'event': event, 'id': event_id, 'data': '\n'.join(data)}
                    data, event = [], ''
                elif not line.startswith(':'):
                    field, separator, value = line.partition(':')
                    value = value.removeprefix(' ') if separator else ''
                    if field == 'data': data.append(value)
                    if field == 'event': event = value
                    if field == 'id' and '\0' not in value: event_id = value
        decoder.decode(b'', final=True)
    finally:
        chunks.close()
wire = ': heartbeat\r\nid: one\r\nevent: response.output_text.delta\r\ndata: {"delta":"xin chào"}\r\n\r\ndata: first\ndata: second\n\ndata: [DONE]\n\n'.encode()
closed = []
def chunks():
    try:
        for byte in wire: yield bytes([byte])
    finally: closed.append(True)
frames = list(sse_events(chunks()))
assert frames == [
    {'event': 'response.output_text.delta', 'id': 'one', 'data': '{"delta":"xin chào"}'},
    {'event': '', 'id': 'one', 'data': 'first\nsecond'},
    {'event': '', 'id': 'one', 'data': '[DONE]'},
]
assert json.loads(frames[0]['data'])['delta'] == 'xin chào'
assert closed == [True]
events = sse_events(chunks())
assert next(events)['event'] == 'response.output_text.delta'
events.close()
assert closed == [True, True]
assert list(sse_events(iter_fixture := (item for item in [b'data: unfinished']))) == []
with connect('ws://127.0.0.1:' + sys.argv[1] + '/v1/responses', additional_headers={'Authorization': 'Bearer fixture-only-token'}, open_timeout=5, close_timeout=5) as socket:
    socket.send(json.dumps({'type': 'response.create', 'model': 'fixture-model', 'input': 'Hello'}))
    assert json.loads(socket.recv(timeout=5))['type'] == 'response.created'
    assert json.loads(socket.recv(timeout=5))['type'] == 'response.output_text.delta'
    socket.send(json.dumps({'type': 'response.cancel'}))
    assert json.loads(socket.recv(timeout=5))['type'] == 'response.cancelled'
    socket.close(1000)
assert socket.close_code == 1000
print('Python manual SSE incremental UTF-8/framing/cancellation and WebSocket create/cancel/close fixtures PASS')
`
