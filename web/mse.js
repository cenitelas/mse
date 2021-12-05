"use strict"

class Mse {
    constructor(url, video, live,events) {
        this.mediaSource = null;
        this.webSocket = null;
        this.timeOut = Date.now()
        this.timeOutInterval = null
        this.mimeCodec = '';
        this.buffer = [];
        this.isLive = live
        this.url = url;
        this.video = video;
        this.pageCreate = "" // "vr.getPageName() ?? "";"
        this.connected = false;
        this.events = events
        this.bufferUpdate = true
        this.pendingRemoveRanges = [];
        window.addEventListener("change_page",async (event)=>{
            await this.stop()
        });
        if (!Uint8Array.prototype.slice) {
            Object.defineProperty(Uint8Array.prototype, 'slice', {
                'enumerable': false,
                'value': function (begin, end) {
                    return new Uint8Array(Array.prototype.slice.call(this, begin, end));
                }
            });
        }
    }

    restart = async () => {
        await this.stop();
        await this.start()
    };

    start = () => {
        clearInterval(this.timeOutInterval)
        this.timeOutWorker()
        this.webSocket = new WebSocket(this.url);
        this.webSocket.binaryType = "arraybuffer";
        this.webSocket.addEventListener('open', this.onOpen);
        this.webSocket.addEventListener('close', this.onClose);
        this.webSocket.addEventListener('message', this.onMessage);
    };

    pause = ()=>{
        console.log("PAUSE")
    }

    stop = async () => {
        await this.stopBuffer()
        clearInterval(this.timeOutInterval)
        if (this.connected) {
            this.connected = false;
            this.webSocket.close();
            this.webSocket.removeEventListener('message', this.onMessage);
            this.webSocket.removeEventListener('open', this.onOpen);
            this.webSocket.removeEventListener('open', this.onClose);
            this.webSocket = null;
        }
        this.connected = false;
    };

    stopBuffer = async () => {
        this.bufferUpdate = true;
        if (this.mediaSource) {
            if (this.mediaSource.sourceBuffers.length > 0) {
                let sb = this.mediaSource.sourceBuffers[0]
                if (!sb.updating) {
                    this.mediaSource.removeSourceBuffer(sb);
                }else{
                    sb.abort()
                }

                sb.removeEventListener("updateend", this.onUpdateEnd)
            }
            this.mediaSource.removeEventListener("segment", this.onSegment);
            this.mediaSource.removeEventListener('sourceopen', this.onSourceOpen);
            this.mediaSource = null;
        }
    }


    onOpen = () => {
        this.connected = true;
        console.log(`Connect ${this.url}`);
    };

    onClose = () => {
            console.log(`Close ${this.url}`);
    };

    getBox = (arr, i)=>{ // input Uint8Array, start index
        return this.toInt(arr, i)
    }

    toInt = (arr, index)=>{ // From bytes to big-endian 32-bit integer.  Input: Uint8Array, index
        var dv = new DataView(arr.buffer, 0);
        return dv.getInt32(index, false); // big endian
    }

    onMessage = async (event) => {
        this.timeOut = Date.now()
        let data = new Uint8Array(event.data);

        if (data[0] === 9) {
            let decoded_arr = data.slice(1);
            this.buffer[this.mimeCodec] = null;
            this.mimeCodec = new TextDecoder("utf-8").decode(decoded_arr);
            if (!MediaSource.isTypeSupported('video/mp4; codecs="' + this.mimeCodec  + '"')) {
                console.log(this.mimeCodec + " is not supported");
                this.stop();
                return;
            }
            await this.stopBuffer();
            await this.replay();
        } else {
            if (!this.buffer[this.mimeCodec]) {
                this.buffer[this.mimeCodec] = [];
            }
            this.buffer[this.mimeCodec].push(data);
            if (this.mediaSource && this.mediaSource.sourceBuffers.length > 0) {
                this.mediaSource.dispatchEvent(new Event('segment'));
            }else{
                this.replay();
            }
        }
    };

    replay = async () => {
        try {
            await this.video.pause();
        } catch (e) {
        }
        this.mediaSource = null;
        this.mediaSource = new MediaSource();
        this.mediaSource.addEventListener('sourceopen', this.onSourceOpen,false);
        this.mediaSource.addEventListener("segment", this.onSegment);
        this.video.src = await URL.createObjectURL(this.mediaSource);
        this.video.onpause = (e)=>{
            this.urlToCurrentTime();
            this.stop()
        }
        this.video.onplay = (e)=>{
            if(this.video.readyState===4){
                this.start();
            }
        }
        try {
            await this.video.play();
        } catch (error) {
        }
    };

    onSourceOpen = () => {
        if (this.mediaSource.sourceBuffers.length > 0) {
            let sb = this.mediaSource.sourceBuffers[0]
            if (!sb.updating) {
                this.mediaSource.removeSourceBuffer(sb);
            }else{
                sb.abort()
            }

            sb.removeEventListener("updateend", this.onUpdateEnd)
        }else {
            this.bufferUpdate = false;
            let sb = this.mediaSource.addSourceBuffer('video/mp4; codecs="' + this.mimeCodec + '"');
            sb.mode = "sequence";
            sb.addEventListener("updateend", this.onUpdateEnd)
            this.events.afterStart()
        }
    };

    onSegment = async () => {
        if (this.buffer[this.mimeCodec].length > 0 && !this.bufferUpdate) {
            if (this.mediaSource.sourceBuffers.length > 0) {
                // this.cleanupSourceBuffer()
                let segment = this.buffer[this.mimeCodec].shift();
                try {
                    let sb = this.mediaSource.sourceBuffers[0]
                    if (!sb.updating) {
                        sb.appendBuffer(segment);
                    }else
                        this.buffer[this.mimeCodec].unshift(segment);
                } catch (error) {
                    this.buffer[this.mimeCodec].unshift(segment);
                    await this.stopBuffer();
                    await this.replay();
                }
            }
        }
    };

    onUpdateEnd = () => {
        if(this.mediaSource.readyState==="open") {
            if(this.buffer.length>1) {
                this.buffer = [this.buffer.pop()]
                this.mediaSource.endOfStream();
                this.video.play();
            }else{
                this.cleanupSourceBuffer()
            }

        }
    };

    cleanupSourceBuffer = () => {
        if (!this.video || !this.mediaSource) {
            return;
        }

        let currentTime = this.video.currentTime;

        const sb = this.mediaSource.sourceBuffers[0];
        if (sb) {
            const buffered = sb.buffered;
            let doRemove = false;

            for (let i = 0; i < buffered.length; i++) {
                const start = buffered.start(i);
                const end = buffered.end(i);

                if (this.isLive && currentTime < end - 2) {
                    this.video.currentTime = currentTime = end;
                    console.log('clock adjusted Peddind:'+this.pendingRemoveRanges.length+" Buffer:"+this.buffer.length);
                }

                if (start <= currentTime && currentTime < end + 3) {
                    if (currentTime - start >= 5) {
                        doRemove = true;
                        this.pendingRemoveRanges.push({start: start, end: currentTime - 3});
                    }
                }
                else if (end < currentTime) {
                    doRemove = true;
                    this.pendingRemoveRanges.push({start: start, end: end});
                }
            }
            if (this.pendingRemoveRanges.length > 5) {
                doRemove = true;
            }
            if (doRemove && !sb.updating) {
                if (!sb.updating) {
                    while (this.pendingRemoveRanges.length && !sb.updating) {
                        const range = this.pendingRemoveRanges.shift();
                        // console.log('remove buff', range.start, range.end, currentTime);

                        if (range.end > currentTime) {
                            this.video.currentTime = range.end;
                        }

                        sb.remove(range.start, range.end);
                    }
                }
            }
        }
    };

    timeOutWorker = async ()=>{
        this.timeOutInterval = setInterval(()=>{
            let diff = Math.abs(new Date() - this.timeOut);
            if (!this.connected){
                clearInterval(this.timeOutInterval)
                return
            }
            if (diff>10000){
                this.urlToCurrentTime();
                this.start()
                console.log(`Reconnect ${this.url}`);
                clearInterval(this.timeOutInterval)
            }
        },10000)
    }

    urlToCurrentTime = ()=>{
        let split = this.url.split("/")
        try {
            Date.parse(split[split.length - 1])
            split[split.length-1] = tester.$refs.timeline.getCurrentTime().toISOString();
        }catch (e){}
        this.url  = split.join("/")
    }
}
