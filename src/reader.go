package fzf

import (
	"bufio"
	"io"
	"sync/atomic"
	"time"

	"github.com/guoshimin/fzf/src/util"
)

// Reader reads from command or standard input
type Reader struct {
	pusher   func([]byte) bool
	eventBox *util.EventBox
	delimNil bool
	event    int32
}

// NewReader returns new Reader object
func NewReader(pusher func([]byte) bool, eventBox *util.EventBox, delimNil bool) *Reader {
	return &Reader{pusher, eventBox, delimNil, int32(EvtReady)}
}

func (r *Reader) startEventPoller() {
	go func() {
		ptr := &r.event
		pollInterval := readerPollIntervalMin
		for {
			if atomic.CompareAndSwapInt32(ptr, int32(EvtReadNew), int32(EvtReady)) {
				r.eventBox.Set(EvtReadNew, true)
				pollInterval = readerPollIntervalMin
			} else if atomic.LoadInt32(ptr) == int32(EvtReadFin) {
				return
			} else {
				pollInterval += readerPollIntervalStep
				if pollInterval > readerPollIntervalMax {
					pollInterval = readerPollIntervalMax
				}
			}
			time.Sleep(pollInterval)
		}
	}()
}

func (r *Reader) fin(success bool) {
	atomic.StoreInt32(&r.event, int32(EvtReadFin))
	r.eventBox.Set(EvtReadFin, success)
}

// ReadSource reads data from the default command or from standard input
func (r *Reader) ReadSource(in io.Reader) {
	r.startEventPoller()
	r.feed(in)
	r.fin(true)
}

func (r *Reader) feed(src io.Reader) {
	delim := byte('\n')
	if r.delimNil {
		delim = '\000'
	}
	reader := bufio.NewReaderSize(src, readerBufferSize)
	for {
		// ReadBytes returns err != nil if and only if the returned data does not
		// end in delim.
		bytea, err := reader.ReadBytes(delim)
		byteaLen := len(bytea)
		if byteaLen > 0 {
			if err == nil {
				// get rid of carriage return if under Windows:
				if util.IsWindows() && byteaLen >= 2 && bytea[byteaLen-2] == byte('\r') {
					bytea = bytea[:byteaLen-2]
				} else {
					bytea = bytea[:byteaLen-1]
				}
			}
			if r.pusher(bytea) {
				atomic.StoreInt32(&r.event, int32(EvtReadNew))
			}
		}
		if err != nil {
			break
		}
	}
}

func (r *Reader) readFrom(in io.Reader) bool {
	r.feed(in)
	return true
}

func (r *Reader) readFromCommand(shell string, cmd string) bool {
	listCommand := util.ExecCommandWith(shell, cmd)
	out, err := listCommand.StdoutPipe()
	if err != nil {
		return false
	}
	err = listCommand.Start()
	if err != nil {
		return false
	}
	r.feed(out)
	return listCommand.Wait() == nil
}
