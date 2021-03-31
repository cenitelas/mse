class Mse {
    constructor(url,video,live) {
        this.url = url
        this.ms = null;
        this.sourceBuffer = null
        this.ws = null;
        this.mimeCodec = ""
        this.live = live
        this.video = video
        this.buffer = []
        if (!Uint8Array.prototype.slice) {
            Object.defineProperty(Uint8Array.prototype, 'slice', {
                value: function (begin, end) {
                    return new Uint8Array(Array.prototype.slice.call(this, begin, end));
                }
            });
        }
    }

    destroy = ()=>{
        this.mimeCodec = ""
        this.buffer = []
        this.sourceBuffer = null
        this.ws?.close()
    }

    play  = ()=>{
        this.connect()
    }

    run = async ()=>{
        await this.video.pause()
        if (this.ms) {
            this.ms.removeEventListener('sourceopen', this.loadMedia, false)
        }
        this.ms = new MediaSource()
        this.ms.addEventListener('sourceopen', this.loadMedia ,false);
        this.video.src = await URL.createObjectURL(this.ms)
        try {
            await this.video.play();
        }catch (e){}
    }

    pushPacket = async ()=>{
        if (this.live){
            try {
                if (!this.sourceBuffer.updating) {
                    this.sourceBuffer.appendBuffer(this.buffer[this.mimeCodec].shift());
                    let buffered = this.video.buffered
                    if (buffered.length > 0) {
                        let end = buffered.end(0)
                        if (end - this.video.currentTime > 0.25) {
                            this.video.currentTime = end - 0.2
                        }
                    }
                }
            } catch (e) {
                console.log(e)
            }
        }else {
            if (this.buffer[this.mimeCodec].length > 0) {
                let segment = this.buffer[this.mimeCodec].shift()
                try {
                    this.sourceBuffer.appendBuffer(segment);
                } catch (e) {
                    console.log(e)
                    this.buffer[this.mimeCodec].unshift(segment)
                    if (this.sourceBuffer.mimeCodec !== this.mimeCodec) {
                        await this.run()
                    }
                }
            }
        }
    }


    connect = (url)=>{
        this.destroy()
        this.ws = new WebSocket(url ?? this.url);
        this.ws.binaryType = "arraybuffer";
        this.ws.onopen = function(event) {
            console.log('Connect ' + this.url);
        }
        this.ws.onmessage = async (event)=>{
            let data = new Uint8Array(event.data);
            if (data[0] === 9) {
                let decoded_arr=data.slice(1);
                this.buffer[this.mimeCodec] = null
                this.mimeCodec = new TextDecoder("utf-8").decode(decoded_arr);
                if (!MediaSource.isTypeSupported('video/mp4; codecs="' + this.mimeCodec + '"')){
                    console.log(this.mimeCodec+ " is not supported")
                    this.destroy()
                    return
                }

                this.sourceBuffer = null;
                await this.run()

            } else {
                if (!this.buffer[this.mimeCodec]){
                    this.buffer[this.mimeCodec] = []
                }
                this.buffer[this.mimeCodec].push(data)
                if (this.sourceBuffer) {
                    this.sourceBuffer.dispatchEvent(new Event('segment'));
                }
            }
        };
    }

    loadMedia = async ()=>{
        this.sourceBuffer = this.ms.addSourceBuffer('video/mp4; codecs="' + this.mimeCodec + '"');
        this.sourceBuffer.mimeCodec = this.mimeCodec
        this.sourceBuffer.mode = "segments"
        this.sourceBuffer.addEventListener("segment",this.pushPacket)
        this.sourceBuffer.addEventListener("updateend", () => {
            if (this.live) {

            }
        })

    }

}

