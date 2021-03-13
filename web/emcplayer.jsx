class Emcplayer {
    constructor(url,video, bufferEnable) {
        this.url = url
        this.bufferEnable = bufferEnable;
        this.streamingStarted = false;
        this.ms = new MediaSource();
        this.sourceBuffer = null
        this.queue = [];
        this.ws = null;
        this.timeSeek = null;
        this.video = video
        if (!Uint8Array.prototype.slice) {
            Object.defineProperty(Uint8Array.prototype, 'slice', {
                value: function (begin, end) {
                    return new Uint8Array(Array.prototype.slice.call(this, begin, end));
                }
            });
        }
    }

    destroy (){
        clearInterval(this.timeSeek)
        this.ws.close()
    }

    play  = ()=>{
        this.ms.addEventListener('sourceopen', this.opened, false);
        this.video.src = window.URL.createObjectURL(this.ms)
        this.video.play();
    }



    pushPacket = (arr)=>{
        let data = arr;
        if(this.bufferEnable) {

            if (!this.streamingStarted) {
                this.sourceBuffer.appendBuffer(data);
                this.streamingStarted = true;
                return;
            }
            this.queue.push(data);
            if (!this.sourceBuffer.updating) {
                this.loadPacket();
            }

            // if (this.queue.length > 10){
            //     this.queue.shift()
            // }
        }else{

            if(!this.sourceBuffer){
                console.log("Error buffer")
                this.destroy()
                return
            }
            try {
                this.sourceBuffer.appendBuffer(data);
                this.streamingStarted = true;
                if (this.video.buffered.length > 1) {
                    console.log("Seek")
                    let end = this.video.buffered.end (0)
                    if (end-this.video.currentTime> 0.15) {
                        this.video.currentTime = end-0.1
                    }
                }
            }catch (e){
                console.log(e)
                this.destroy()
                this.play()
            }
        }
    }

    loadPacket = ()=>{
        if (!this.sourceBuffer.updating) {
            if (this.queue.length > 0) {
                let inp = this.queue.shift();
                // this.queue = inp;
                this.sourceBuffer.appendBuffer(inp);
            } else {
                this.streamingStarted = false;
            }
        }
    }

    opened = ()=>{
        this.ws = new WebSocket(this.url);
        this.ws.binaryType = "arraybuffer";
        this.ws.onopen = function(event) {
            console.log('Connect ' + this.url);
        }

        this.ws.onmessage = (event)=>{

            let data = new Uint8Array(event.data);
            if (data[0] === 9) {
                let decoded_arr=data.slice(1);
                let mimeCodec = null;
                if (window.TextDecoder) {
                    mimeCodec = new TextDecoder("utf-8").decode(decoded_arr);
                } else {
                    mimeCodec = this.Utf8ArrayToStr(decoded_arr);
                }
               if (!MediaSource.isTypeSupported('video/mp4; codecs="' + mimeCodec + '"')){
                   console.log(mimeCodec+ " is not supported")
                   this.destroy()
                   return
               }

                this.sourceBuffer = this.ms.addSourceBuffer('video/mp4; codecs="' + mimeCodec + '"');
                this.sourceBuffer.mode = "segments"
                this.sourceBuffer.addEventListener("updateend", this.loadPacket);
            } else {
                this.pushPacket(event.data);
            }
        };
    }


    Utf8ArrayToStr = (array)=>{
        let out, i, len, c;
        let char2, char3;
        out = "";
        len = array.length;
        i = 0;
        while (i < len) {
            c = array[i++];
            switch (c >> 4) {
                case 7:
                    out += String.fromCharCode(c);
                    break;
                case 13:
                    char2 = array[i++];
                    out += String.fromCharCode(((c & 0x1F) << 6) | (char2 & 0x3F));
                    break;
                case 14:
                    char2 = array[i++];
                    char3 = array[i++];
                    out += String.fromCharCode(((c & 0x0F) << 12) |
                        ((char2 & 0x3F) << 6) |
                        ((char3 & 0x3F) << 0));
                    break;
            }
        }
        return out;
    }
}

