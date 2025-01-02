//go:build js && wasm

package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"strconv"
	"sync/atomic"
	"syscall/js"
	"time"

	ngrok_log "golang.ngrok.com/ngrok/log"

	"github.com/coder/websocket"
	"golang.ngrok.com/ngrok"
	"golang.ngrok.com/ngrok/config"
)

func init() {
	ngrok.SetWasm(true)
}

// Simple logger that forwards to the Go standard logger.
type logger struct {
	lvl ngrok_log.LogLevel
}

func (l *logger) Log(ctx context.Context, lvl ngrok_log.LogLevel, msg string, data map[string]interface{}) {
	if lvl > l.lvl {
		return
	}
	lvlName, _ := ngrok_log.StringFromLogLevel(lvl)
	log.Printf("[%s] %s %v", lvlName, msg, data)
}

var l *logger = &logger{
	lvl: ngrok_log.LogLevelDebug,
}

func main() {
	// return
	c := make(chan struct{})

	// Register functions
	js.Global().Set("ngrokListenAndForward", js.FuncOf(listenAndForward))

	<-c // Keep running
}

func listenAndForward(this js.Value, args []js.Value) interface{} {
	opts := args[0]
	fmt.Println("opts", opts)
	addr := opts.Get("addr")
	authtoken := opts.Get("authtoken")
	hostname := opts.Get("hostname")
	fmt.Println("addr", addr)
	fmt.Println("authtoken", authtoken)
	fmt.Println("hostname", hostname)

	// Create promise constructor and resolver
	promise := js.Global().Get("Promise")
	handler := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		resolve := args[0]
		reject := args[1]

		go func() {
			connChan := make(chan struct {
				conn net.Conn
				err  error
			}, 1)

			ctx := context.Background()

			go func() {
				// https://expo-sb-worker.sb-labs-staging.workers.dev
				conn, err := dialWebsocket(ctx, "wss://expo-sb-worker.sb-labs-staging.workers.dev")
				// conn, err := dialWebsocket(ctx, "ws://localhost:8787")
				if err != nil {
					fmt.Println("err", err)
				}
				connChan <- struct {
					conn net.Conn
					err  error
				}{conn, err}
			}()
			connResult := <-connChan

			addrUrl, err := url.Parse(addr.String())
			if err != nil {
				panic("unable to parse backend url")
			}

			// array of httpEndpointOptions
			httpOpts := []config.HTTPEndpointOption{}
			if !hostname.IsUndefined() {
				httpOpts = append(httpOpts, config.WithDomain(hostname.String()))
			}
			fwd, err := ngrok.ListenAndForward(
				context.Background(),
				addrUrl,
				config.HTTPEndpoint(httpOpts...),
				ngrok.WithAuthtoken(authtoken.String()),
				ngrok.WithDialer(&websocketDialer{conn: connResult.conn}),
				// ngrok.WithServer(server_addr),
			)

			if err != nil {
				reject.Invoke(err.Error())
				return
			}

			l.Log(ctx, ngrok_log.LogLevelInfo, "ingress established", map[string]any{
				"url": fwd.URL(),
			})

			resolve.Invoke(fwd.URL())
		}()

		return nil
	})
	defer handler.Release()

	return promise.New(handler)
}

// Helper to return JS objects
func wrap(val interface{}, err error) map[string]interface{} {
	result := make(map[string]interface{})
	if err != nil {
		result["error"] = err.Error()
		return result
	}
	result["value"] = val
	return result
}

func dialWebsocket(ctx context.Context, urlStr string) (net.Conn, error) {
	c, res, err := websocket.Dial(ctx, urlStr, &websocket.DialOptions{
		// Subprotocols: []string{"derp"},
	})
	if err != nil {
		fmt.Printf("websocket Dial: %v, response: %+v", err, res)
		return nil, err
	}

	netConn := websocket.NetConn(ctx, c, websocket.MessageBinary)
	return netConn, nil
}

type websocketDialer struct {
	conn net.Conn
}

var id uint32 = 0

// Command types matching the server implementation
const (
	CmdConnect   uint8 = 1
	CmdConnected uint8 = 2
	CmdData      uint8 = 3
	CmdClose     uint8 = 4
	CmdError     uint8 = 5
)

func (d *websocketDialer) Dial(network, address string) (net.Conn, error) {
	currentID := atomic.AddUint32(&id, 1) - 1
	conn := tcpOverWebsocket(d.conn, currentID)
	return conn, nil
}

func (d *websocketDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	currentID := atomic.AddUint32(&id, 1) - 1
	// Parse address into host and port
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, err
	}

	// Create connect frame: [1:cmd][4:connId][2:port][n:address]
	addressBytes := []byte(host)
	frame := make([]byte, 7+len(addressBytes))

	// Write command
	frame[0] = CmdConnect

	// Write connection ID (big endian)
	binary.BigEndian.PutUint32(frame[1:5], currentID)

	// Write port (big endian)
	binary.BigEndian.PutUint16(frame[5:7], uint16(port))

	// Write address
	copy(frame[7:], addressBytes)

	// Send frame
	_, err = d.conn.Write(frame)
	if err != nil {
		return nil, err
	}

	conn := tcpOverWebsocket(d.conn, currentID)
	return conn, nil
}

// var id = 0

// func (d *websocketDialer) Dial(network, address string) (net.Conn, error) {
// 	currentID := id
// 	conn := tcpOverWebsocket(d.conn, currentID)
// 	id++
// 	return conn, nil
// }

// func (d *websocketDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
// 	currentID := id
// 	jsonFrame, err := json.Marshal(Frame{Method: "start", ID: currentID, URL: address})
// 	if err != nil {
// 		return nil, err
// 	}
// 	d.conn.Write(jsonFrame)
// 	conn := tcpOverWebsocket(d.conn, currentID)
// 	id++
// 	return conn, nil
// }

type wsConn struct {
	conn    net.Conn
	id      uint32
	readBuf []byte
	header  []byte
}

// type Frame struct {
// 	Method string `json:"method"`
// 	ID     int    `json:"id"`
// 	URL    string `json:"url"`
// 	Data   []int  `json:"data"`
// }

// // packets for reads
// type Packet struct {
// 	Method string `json:"method"`
// 	ID     int    `json:"id"`
// 	Data   []int  `json:"data"`
// }

// func (w *wsConn) Read(b []byte) (n int, err error) {
// 	fmt.Println("before Read", b)
// 	log.Println(string(debug.Stack()))
// 	n, err = w.conn.Read(b)
// 	fmt.Println("n ", n, err)

// 	if err != nil {
// 		return 0, err
// 	}
// 	return n, nil
// }

func (w *wsConn) Read(b []byte) (n int, err error) {
	// If we still have data in the buffer from a previous read
	if len(w.readBuf) > 0 {
		n = copy(b, w.readBuf)
		w.readBuf = w.readBuf[n:]
		return n, nil
	}

	// Read the header (5 bytes: 1 byte command + 4 bytes connection ID)
	header := make([]byte, 5)
	_, err = io.ReadFull(w.conn, header)
	if err != nil {
		return 0, err
	}

	cmd := header[0]
	frameConnID := binary.BigEndian.Uint32(header[1:5])

	// Verify connection ID
	if frameConnID != w.id {
		return 0, fmt.Errorf("received frame for wrong connection: got %d, want %d", frameConnID, w.id)
	}

	switch cmd {
	case CmdConnected:
		return 0, nil
	case CmdData:
		// For data frames, read the length prefix (4 bytes)
		lenBuf := make([]byte, 4)
		_, err = io.ReadFull(w.conn, lenBuf)
		if err != nil {
			return 0, err
		}
		messageLen := binary.BigEndian.Uint32(lenBuf)
		// Read the full message
		message := make([]byte, messageLen)
		_, err = io.ReadFull(w.conn, message)
		if err != nil {
			return 0, err
		}

		// Copy as much as we can into the provided buffer
		n = copy(b, message)
		// If we couldn't fit the whole message, store the rest in readBuf
		if n < len(message) {
			w.readBuf = message[n:]
		}
		return n, nil

	case CmdClose:
		fmt.Println("CmdClose")
		return 0, io.EOF

	case CmdError:
		// Read 2 bytes error code
		errorCode := make([]byte, 2)
		_, err = io.ReadFull(w.conn, errorCode)
		if err != nil {
			return 0, fmt.Errorf("failed to read error code: %v", err)
		}
		return 0, fmt.Errorf("remote error: code %d", binary.BigEndian.Uint16(errorCode))

	default:
		return 0, fmt.Errorf("unexpected command: %d", cmd)
	}
}

func (w *wsConn) Write(b []byte) (n int, err error) {
	// Create frame: [1:cmd][4:connId][4:length][n:payload]
	frame := make([]byte, 9+len(b))

	// Write command
	frame[0] = CmdData

	// Write connection ID (big endian)
	binary.BigEndian.PutUint32(frame[1:5], w.id)

	// Write length (big endian)
	binary.BigEndian.PutUint32(frame[5:9], uint32(len(b)))

	// Write payload
	copy(frame[9:], b)

	// Send frame
	_, err = w.conn.Write(frame)
	if err != nil {
		return 0, err
	}

	return len(b), nil
}

// Add Close method to properly close the connection
func (w *wsConn) Close() error {
	// Create close frame: [4:cmd][4:connId]
	frame := make([]byte, 5)
	frame[0] = CmdClose
	binary.BigEndian.PutUint32(frame[1:5], w.id)

	_, err := w.conn.Write(frame)
	if err != nil {
		return err
	}
	return nil
}

// func (w *wsConn) Write(b []byte) (n int, err error) {
// 	data := make([]int, len(b))
// 	for i, v := range b {
// 		data[i] = int(v)
// 	}
// 	jsonFrame, err := json.Marshal(Frame{Method: "data", ID: w.id, Data: data})
// 	if err != nil {
// 		return 0, err
// 	}
// 	return w.conn.Write(jsonFrame)
// }

func (w *wsConn) LocalAddr() net.Addr                { return w.conn.LocalAddr() }
func (w *wsConn) RemoteAddr() net.Addr               { return w.conn.RemoteAddr() }
func (w *wsConn) SetDeadline(t time.Time) error      { return w.conn.SetDeadline(t) }
func (w *wsConn) SetReadDeadline(t time.Time) error  { return w.conn.SetReadDeadline(t) }
func (w *wsConn) SetWriteDeadline(t time.Time) error { return w.conn.SetWriteDeadline(t) }

func tcpOverWebsocket(conn net.Conn, id uint32) net.Conn {
	return &wsConn{conn: conn, id: id, header: make([]byte, 5), readBuf: make([]byte, 0)}
}

// [3 0 0 0 0 22 3 3 0 122 2 0 0 118 3 3 83 34 35 140 43 127 14 124 187 131 83 144 0 38 48 150 14 240 134 221 7 5 254 94 49 185 4 86 107 153 158 252 32 14 243 225 113 148 184 61 34 253 130 64 95 55 162 123 6 214 141 174 155 224 117 249 178 31 94 24 31 27 73 102 172 19 3 0 0 46 0 43 0 2 3 4 0 51 0 36 0 29 0 32 3 48 236 27 118 44 105 179 8 64 222 39 111 60 221 133 142 69 54 95 162 74 242 227 106 124 178 153 132 246 170 6 20 3 3 0 1 1 23 3 3 0 23 136 77 226 179 140 80 191 39 20 131 85 179 138 188 239 99 158 39 196 252 218 58 238 23 3 3 10 113 21 212 245 142 36 2 139 24 97 94 126 201 109 125 201 114 124 212 151 185 255 127 185 202 32 129 146 242 195 37 183 224 11 56 242 242 43 74 65 220 184 51 79 147 19 198 33 27 20 231 105 199 23 218 53 134 201 66 106 60 12 177 152 57 111 227 147 143 78 13 116 215 123 219 149 15 16 213 141 97 207 145 71 99 161 64 255 122 113 163 143 62 211 21 185 150 14 191 174 187 44 36 143 219 230 237 1 39 195 226 178 225 246 146 78 199 100 68 253 101 204 164 229 77 224 57 153 32 41 69 157 11 12 127 143 105 184 153 105 84 131 198 108 22 12 18 236 177 158 104 78 241 182 72 158 31 74 252 50 237 102 47 175 206 203 113 100 33 188 198 230 84 45 235 130 92 79 25 52 249 206 237 137 47 105 59 170 26 202 16 7 49 221 10 84 253 59 250 99 57 69 28 251 83 3 15 90 27 150 125 92 78 154 216 167 50 216 71 160 134 128 69 226 33 219 130 240 15 22 130 143 87 218 149 208 247 223 110 147 203 70 190 176 146 210 1 49 239 152 8 246 102 88 137 101 64 254 154 124 216 120 189 196 159 176 144 123 210 166 235 100 214 78 57 171 250 194 131 227 6 82 185 11 3 251 142 179 250 153 144 80 132 217 63 162 48 53 8 9 123 154 16 142 33 113 87 241 228 126 24 219 104 155 22 113 62 92 99 136 149 111 153 29 37 138 196 198 129 136 92 245 139 128 213 221 95 115 30 89 35 175 20 52 247 61 6 69 249 159 230 140 87 225 134 219 4 176 8 190 23 119 85 198 163 167 69 188 48 49 199 167 255 50 92 31 116 6 208 84 196 129 141 27 17 23 63 110 22 57 193 220 246 85 244 108 175 247 85 24 196 226 210 235 56 33 43 116 196 181 156 97 46 160 165 94 86 190 202 87 27 106 229 141 79 151 219 222 81 118 73 177 122 91 145 251 141 230 207 189 245 182 179 98 78 161 250 59 199 22 149 103 77 112 243 66 224 222 255 217 215 11 252 47 224 64 186 218 219 71 82 102 199 148 119 60 192 30 101 184 211 162 158 90 229 67 42 91 221 68 209 88 243 238 156 51 92 88 18 201 85 189 28 188 141 197 75 126 37 134 102 96 7 238 98 204 194 6 19 157 182 83 144 94 197 25 161 7 209 11 241 118 59 134 78 59 235 248 249 228 229 148 49 63 65 176 224 108 177 134 77 220 122 156 215 190 18 186 114 62 94 92 178 217 44 128 198 158 36 84 137 183 79 108 202 11 140 166 187 216 61 195 197 197 72 75 251 9 250 252 72 58 189 220 194 210 220 19 195 195 162 97 232 42 108 63 115 241 202 88 251 131 105 93 209 157 171 99 140 91 94 105 181 183 16 96 46 132 52 135 148 140 156 4 190 236 58 121 202 226 161 110 108 124 169 128 77 176 131 132 3 94 44 40 13 207 185 48 81 196 92 27 214 222 118 128 89 178 109 54 87 171 39 156 26 82 50 3 156 238 160 166 179 0 147 203 27 64 21 83 91 127 149 118 113 96 134 77 89 88 83 175 88 2 109 159 48 204 254 82 76 141 134 69 110 7 207 110 134 12 79 124 246 98 136 152 252 132 245 103 170 106 118 134 44 14 39 53 142 177 112 165 178 121 9 189 111 91 242 141 223 156 243 200 241 181 57 116 77 242 103 49 105 192 18 154 163 53 53 19 132 137 126 195 159 225 131 122 48 111 167 219 2 140 230 185 139 61 128 205 77 135 34 141 159 237 160 68 127 90 163 186 103 15 232 184 222 159 48 19 11 214 193 213 146 118 106 121 180 135 208 22 255 31 149 183 175 77 229 130 204 221 71 242 104 206 139 217 245 133 192 148 224 250 110 17 55 189 150 71 27 187 61 155 42 23 107 199 0 231 121 38 53 224 11 192 1 37 123 163 121 221 84 59 65 112 41 158 125 37 147 153 240 81 74 249 80 48 217 235 163 159 91 17 51 42 157 119 225 37 230 100 245 3 211 178 95 120 252 137 102 168 34 145 43 6 237 210 192 56 153 29 154 178 87 174 59 114 10 72 171 162 86 8 106 145 88 103 67 61 243 222 133 114 208 162 253 116 5 61 20 194 29 22 47 220 179 127 24 167 105 233 140 154 142 240 112 128 194 28 0 14 64 68 105 61 52 224 173 182 21 93 58 216 22 225 161 171 255 85 204 92 34 203 151 140 31 237 40 247 182 174 173 69 175 32 210 178 198 185 169 166 91 193 50 233 46 222 174 128 147 204 242 55 3 128 85 221 255 58 6 22 26 144 13 29 57 229 139 57 177 214 26 238 154 135 80 88 42 88 234 136 86 90 8 67 147 4 149 95 101 121 165 114 67 127 204 26 134 144 23 37 29 164 20 131 17 69 150 161 30 179 243 254 142 217 255 92 180 60 201 30 195 32 203 143 61 172 51 33 57 188 135 230 232 51 11 12 161 145 10 69 152 36 214 93 6 254 68 30 133 238 77 60 125 181 122 226 123 128 97 128 137 167 98 80 234 215 10 29 60 37 195 189 184 104 95 138 167 204 80 158 186 222 220 113 92 163 1 181 109 29 163]
