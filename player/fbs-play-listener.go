package player

import (
	"encoding/binary"

	"io"
	"time"
	"fmt"
	"vncproxy/client"
	"vncproxy/common"
	"vncproxy/logger"
	"vncproxy/server"
)

type VncStreamFileReader interface {
	io.Reader
	CurrentTimestamp() int
	ReadStartSession() (*common.ServerInit, error)
	CurrentPixelFormat() *common.PixelFormat
	Encodings() []common.IEncoding
}

type FBSPlayListener struct {
	Conn             *server.ServerConn
	Fbs              VncStreamFileReader
	serverMessageMap map[uint8]common.ServerMessage
	firstSegDone     bool
	startTime        int
}

func ConnectFbsFile(filename string, conn *server.ServerConn) (*FbsReader, error) {
	fbs, err := NewFbsReader(filename)
	if err != nil {
		logger.Error("failed to open fbs reader:", err)
		return nil, err
	}
	//NewFbsReader("/Users/amitbet/vncRec/recording.rbs")
	initMsg, err := fbs.ReadStartSession()
	//TAF-timing note: ^gets all the headers but not time yet into the fbsreader
	if err != nil {
		logger.Error("failed to open read fbs start session:", err)
		return nil, err
	}
	conn.SetPixelFormat(&initMsg.PixelFormat)
	conn.SetHeight(initMsg.FBHeight)
	conn.SetWidth(initMsg.FBWidth)
	conn.SetDesktopName(string(initMsg.NameText))

	return fbs, nil
}

func NewFBSPlayListener(conn *server.ServerConn, r *FbsReader) *FBSPlayListener {
	h := &FBSPlayListener{Conn: conn, Fbs: r}
	cm := client.MsgBell(0)
	h.serverMessageMap = make(map[uint8]common.ServerMessage)
	h.serverMessageMap[0] = &client.MsgFramebufferUpdate{}
	h.serverMessageMap[1] = &client.MsgSetColorMapEntries{}
	h.serverMessageMap[2] = &cm
	h.serverMessageMap[3] = &client.MsgServerCutText{}

	return h
}
func (handler *FBSPlayListener) Consume(seg *common.RfbSegment) error {
	//TAF_timing note: this handles the client messages.
	switch seg.SegmentType {
	case common.SegmentFullyParsedClientMessage:
		clientMsg := seg.Message.(common.ClientMessage)
		logger.Debugf("ClientUpdater.Consume:(vnc-server-bound) got ClientMessage type=%s", clientMsg.Type())
		switch clientMsg.Type() {

		case common.FramebufferUpdateRequestMsgType:
			if !handler.firstSegDone {
				handler.firstSegDone = true
				handler.startTime = int(time.Now().UnixNano() / int64(time.Millisecond))
				//TAF timing note
				//so if it's the FIRST segment (client just connected) then
				//we 'start'
			}
			handler.sendFbsMessage()
			//TAF send the goodies
		}
		// server.MsgFramebufferUpdateRequest:
	}
	return nil
}

func (h *FBSPlayListener) sendFbsMessage() {
	var messageType uint8
	//messages := make(map[uint8]common.ServerMessage)
	fbs := h.Fbs
	//conn := h.Conn
	err := binary.Read(fbs, binary.BigEndian, &messageType)
	if err != nil {
		logger.Error("TestServer.NewConnHandler: Error in reading FBS segment: ", err)
		return
	}
	//common.IClientConn{}
	binary.Write(h.Conn, binary.BigEndian, messageType)
	msg := h.serverMessageMap[messageType]
	if msg == nil {
		logger.Error("TestServer.NewConnHandler: Error unknown message type: ", messageType)
		return
	}
	timeSinceStart := int(time.Now().UnixNano()/int64(time.Millisecond)) - h.startTime
	//TAF - ^this is time since client connect
	timeToSleep := fbs.CurrentTimestamp() - timeSinceStart
	//TAF currenttimestamp is the READER time
	if timeToSleep > 0 {
		logger.Error("ShouldSleep " + fmt.Sprintf("%v",timeToSleep))
		if (timeToSleep > (10000) ) {
			timeToSleep=300
			logger.Error("SKIPPING")
		}
		time.Sleep(time.Duration(timeToSleep) * time.Millisecond)


	}
	//TAF is this as simple as just removing the sleep call?
	//Well, yes....but then you lose the timestamp data.

	//so have the scripted-client (modified 'vncscreenshot') making requests at
	// (e.g.) 10 FPS and then remove sleeps here so that each request will get
	//the next segment, no framebufferupdaterequests will be wasted (not that we care)
	//but also no time will be wasted sleeping serverside.

	//TAF - I guess I will link the screenshot and player progs by the network
	//as they're intended to be.  Technically, I bet there's circumstances that
	//will cause dropped frames or other ugliness in the recording output,
	//but those are probably not an issue on the loopback interface and for reasonable
	//framerates.  Like, less than 1000 FPS...

	//Also....Is this technically NOT a real-time playback?  If the client stops
	//making update requests, do we ever miss any segments?  Nowhere are we scanning
	//segments to "catch up"

	//Which is to say, this player sleeps so you'll never get frameupdates too early
	//but you're more than welcome to get them too late.

	//Whereas in the real world, the framebuffer might be changing without you requesting
	//it, and by making a delayed request you could miss things.

	//So...instead of actually sleeping.   I could do something here to notify
	//the scripted-client how much time passed.  I want the scripted client
	//to inject frames (imagemagick text-to-png simple whatevers) to show this
	//and it'll be cleaner to build the frames there (at the PNG export step)
	//than try to hack these Rectangles that compose FBs updates

	//Should I try to play within these confines and hack in a VNC message type?
	//Or maybe I can be quicker by just connecting the two programs on an entirely separate
	//port (or filesystem thing or whatever) to get the client to be aware of
	//delays

	//Okay it's ugly but I can just write to a tmpfile for each response with
	//the delay that would've been there (iff it's more than (eg) 10 seconds)
	//and then the screenshot reader can, after making the request and getting
	//a response, also know that the tmpfile has been updated with the #of ms

	//ugly, but it will save me needing more from a structural undertanding of
	//this code.  I guess we're not making a pull request with the video-extract
	//feature...  Maybe later.

	err = msg.CopyTo(fbs, h.Conn, fbs)
	if err != nil {
		logger.Error("TestServer.NewConnHandler: Error in reading FBS segment: ", err)
		return
	}
}
