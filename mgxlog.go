package mgxlog

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

func NewMgxLog(path string, fileSize, fileCount, flushSecond, flushLines int) (*MgxLog, error) {
	os.MkdirAll(path, 0666)
	fl, _ := os.ReadDir(path)
	ml := &MgxLog{findex: 0, fileSize: fileSize, fileCount: fileCount, flushSecond: flushSecond, flushLines: flushLines}
	for _, fi := range fl {
		if !fi.IsDir() {
			strs := strings.Split(fi.Name(), ".")
			i, _ := strconv.Atoi(strs[len(strs)-1])
			if i > 0 {
				if ml.findex < i {
					ml.findex = i
				}
			}
		}
	}
	f, err := os.OpenFile(path+"run.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		return nil, err
	}
	ml.f = f
	ml.p = path
	ml.fchan = make(chan string, 100)
	ml.exitchan = make(chan int)
	go ml.start()
	return ml, nil
}

const (
	LDefault = 1 << iota
	LStdOnly
	LFileOnly
)

type MgxLog struct {
	mu          sync.Mutex
	f           *os.File
	p           string //日志目录名
	fchan       chan string
	findex      int
	exitchan    chan int
	fileSize    int //日志文件大小
	fileCount   int //保留文件数
	flushSecond int //刷新秒数
	flushLines  int //刷新行数
}

func (ml *MgxLog) Info(v ...interface{}) {
	ml.out("INFO", LDefault, v...)
}

func (ml *MgxLog) InfoStd(v ...interface{}) {
	ml.out("INFO", LStdOnly, v...)
}

func (ml *MgxLog) InfoFile(v ...interface{}) {
	ml.out("INFO", LFileOnly, v...)
}

func (ml *MgxLog) Infof(f string, v ...interface{}) {
	ml.outf("INFO", LDefault, f, v...)
}

func (ml *MgxLog) InfoStdf(f string, v ...interface{}) {
	ml.outf("INFO", LStdOnly, f, v...)
}
func (ml *MgxLog) InfoFilef(f string, v ...interface{}) {
	ml.outf("INFO", LFileOnly, f, v...)
}

func (ml *MgxLog) Error(v ...interface{}) {
	ml.out("ERROR", LDefault, v...)
}

func (ml *MgxLog) ErrorStd(v ...interface{}) {
	ml.out("ERROR", LStdOnly, v...)
}
func (ml *MgxLog) ErrorFile(v ...interface{}) {
	ml.out("ERROR", LFileOnly, v...)
}

func (ml *MgxLog) Errorf(f string, v ...interface{}) {
	ml.outf("ERROR", LDefault, f, v...)
}
func (ml *MgxLog) ErrorStdf(f string, v ...interface{}) {
	ml.outf("ERROR", LStdOnly, f, v...)
}
func (ml *MgxLog) ErrorFilef(f string, v ...interface{}) {
	ml.outf("ERROR", LFileOnly, f, v...)
}

func (ml *MgxLog) Warn(v ...interface{}) {
	ml.out("WARN", LDefault, v...)
}

func (ml *MgxLog) WarnStd(v ...interface{}) {
	ml.out("WARN", LStdOnly, v...)
}
func (ml *MgxLog) WarnFile(v ...interface{}) {
	ml.out("WARN", LFileOnly, v...)
}

func (ml *MgxLog) Warnf(f string, v ...interface{}) {
	ml.outf("WARN", LDefault, f, v...)
}
func (ml *MgxLog) WarnStdf(f string, v ...interface{}) {
	ml.outf("WARN", LStdOnly, f, v...)
}
func (ml *MgxLog) WarnFilef(f string, v ...interface{}) {
	ml.outf("WARN", LFileOnly, f, v...)
}

func (ml *MgxLog) Debug(v ...interface{}) {
	ml.out("DEBUG", LDefault, v...)
}

func (ml *MgxLog) DebugStd(v ...interface{}) {
	ml.out("DEBUG", LStdOnly, v...)
}
func (ml *MgxLog) DebugFile(v ...interface{}) {
	ml.out("DEBUG", LFileOnly, v...)
}

func (ml *MgxLog) Debugf(f string, v ...interface{}) {
	ml.outf("DEBUG", LDefault, f, v...)
}
func (ml *MgxLog) DebugStdf(f string, v ...interface{}) {
	ml.outf("DEBUG", LStdOnly, f, v...)
}
func (ml *MgxLog) DebugFilef(f string, v ...interface{}) {
	ml.outf("DEBUG", LFileOnly, f, v...)
}

func (ml *MgxLog) outf(lx string, stat int, f string, v ...interface{}) {
	str := ml.getHead(lx) + fmt.Sprintf(f, v...) + "\r\n"
	if stat == LDefault || stat == LStdOnly {
		ml.mu.Lock()
		os.Stderr.WriteString(str)
		ml.mu.Unlock()
	}
	if stat == LDefault || stat == LFileOnly {
		ml.fchan <- str
	}
}
func (ml *MgxLog) out(lx string, stat int, v ...interface{}) {
	str := ml.getHead(lx) + fmt.Sprintln(v...)
	if stat == LDefault || stat == LStdOnly {
		ml.mu.Lock()
		os.Stderr.WriteString(str)
		ml.mu.Unlock()
	}
	if stat == LDefault || stat == LFileOnly {
		ml.fchan <- str
	}
}

func (ml *MgxLog) start() {
	sb := bytes.NewBufferString("")
	lines := 0
	for {
		select {
		case str, ok := <-ml.fchan:
			if !ok {
				if sb.Len() > 0 {
					ml.f.Write(sb.Bytes())
				}
				ml.exitchan <- 1
				return
			}
			lines++
			sb.WriteString(str)
			if ml.isChangeFile(sb.Len()) {
				ml.f.Write(sb.Bytes())
				ml.changeFile()
				sb.Reset()
			} else if lines > ml.flushLines {
				ml.f.Write(sb.Bytes())
				sb.Reset()
				ml.f.Sync()
				lines = 0
			}
		case <-time.After(time.Duration(ml.flushSecond) * time.Second):
			if sb.Len() > 0 {
				ml.f.Write(sb.Bytes())
				if ml.isChangeFile(0) {
					ml.changeFile()
				} else {
					ml.f.Sync()
				}
			}
			sb.Reset()
		}
	}
}

func (ml *MgxLog) isChangeFile(sl int) bool {
	fi, err := ml.f.Stat()
	if err != nil {
		return true
	}
	return int(fi.Size())+sl > ml.fileSize
}
func (ml *MgxLog) changeFile() {
	ml.findex++
	ml.f.Sync()
	ml.f.Close()
	os.Rename(ml.p+"run.log", ml.p+"run.log."+strconv.Itoa(ml.findex))
	f, err := os.OpenFile(ml.p+"run.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		return
	}
	ml.f = f
	os.Remove(ml.p + "run.log." + strconv.Itoa(ml.findex-ml.fileCount))
}

func (ml *MgxLog) getHead(qz string) string {
	t := time.Now()
	_, file, line, ok := runtime.Caller(3)
	if !ok {
		file = "???"
		line = 0
	}
	short := file
	for i := len(file) - 1; i > 0; i-- {
		if file[i] == '/' {
			short = file[i+1:]
			break
		}
	}
	year, month, day := t.Date()
	hour, min, sec := t.Clock()
	return fmt.Sprintf("%04d-%02d-%02d %02d:%02d:%02d:%06d", year, month, day, hour, min, sec, t.Nanosecond()/1e3) + "[" + short + ":" + strconv.Itoa(line) + "][" + qz + "]"
}

func (ml *MgxLog) Flush() {
	close(ml.fchan)
	<-ml.exitchan
	ml.f.Sync()
	ml.f.Close()
}
