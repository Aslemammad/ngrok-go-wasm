const go = new Go();

WebAssembly.instantiateStreaming(fetch("/static/ngrok.wasm"), go.importObject)
    .then((result) => {
        go.run(result.instance);
        console.log("result", result);
    })
    .then(async (result) => {
        // const authtoken = "1XoV8Waji8VfVfAmKxW9sdV8jqB_x9GH3hgsF6CiKSUztAfn";
        // const backendURL = "http://localhost:8000";

        // opts {
        //     hostname: '3IrNCRM-anonymous-8081.exp.direct',
        //     authtoken: '5W1bR67GNbWcXqmxZzBG1_56GezNeaX6sSRvn8npeQ8',
        //     configPath: '/Users/mohammadbagherabiyat/.expo/ngrok.yml',
        //     onStatusChange: [Function: onStatusChange],
        //     port: 8081,
        //     proto: 'http',
        //     addr: 8081
        //   }
        const authtoken = "5W1bR67GNbWcXqmxZzBG1_56GezNeaX6sSRvn8npeQ8";
        const backendURL = "http://localhost:8000";
        const hostname = "3IrNCRM-anonymous-8081.exp.direct";

        try {
            const url = await ngrokListenAndForward({ authtoken, addr: backendURL, hostname });

            console.log("url", url);
        } catch (err) {
            console.error("Error:", err);
        }
    });


// type Frame = {
//     method: 'start';
//     id: number;
//     url: string;
// } | {
//     method: 'data';
//     id: number;
//     data: Uint8Array;
// } | {
//     method: 'end';
//     id: number;
// }

// type Packet ={
//     method: 'data';
//     id: number;
//     data: number[];
// }


// const ws = new WebSocket("ws://localhost:8787");

// ws.close()
// let id = 0

// function tcp(url, port) {
//     const currentId = id++
//     const { port1: client, port2: server } = new MessageChannel()

//     ws.send(JSON.stringify({
//         method: 'start',
//         id: currentId,
//         url: `${url}:${port}`,
//     }));

//     ws.addEventListener('message', (event) => {
//         const packet = JSON.parse(event.data);
//         if (packet.method === 'data' && packet.id === currentId) {
//             const data = new Uint8Array(packet.data);
//             server.postMessage(data)
//         } 
//     });

//     server.addEventListener('message', (event) => {
//         console.log("server message", event.data)
//         ws.send(JSON.stringify({
//             method: 'data',
//             id: currentId,
//             data: [...event.data],
//         }));
//     });
//     server.start()
//     client.start()

//     return client
// }

// ws.onopen = () => {
//     const client = tcp('example.com', 80)

//     const GET = `GET / HTTP/1.1
// Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7
// Accept-Language: en-US,en;q=0.9
// Cache-Control: no-cache
// Connection: keep-alive
// Host: example.com
// Pragma: no-cache
// Upgrade-Insecure-Requests: 1
// User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36\r\n\r\n`

//     client.addEventListener('message', (event) => {
//         console.log(new TextDecoder().decode(event.data))
//     })

//     console.log("sending GET")
//     client.postMessage(new TextEncoder().encode(GET))
// }

