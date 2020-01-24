package datadog

import (
	"bytes"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Options define the options for Datadog log stream
type Options struct {
	Source   string
	Service  string
	Hostname string
	Tags     []string
}

// Hook is the struct holding connect information to Datadog backend
type Hook struct {
	host      string
	apiKey    string
	maxRetry  int
	formatter logrus.Formatter
	minLevel  logrus.Level
	options   Options

	ch     chan []byte
	buffer [][]byte
	m      sync.Mutex
	err    error
}

const (
	// DatadogUSHost - Host For Datadog US
	DatadogUSHost = "http-intake.logs.datadoghq.com"
	// DatadogEUHost - Host For Datadog EU
	DatadogEUHost = "http-intake.logs.datadoghq.eu"

	basePath       = "/v1/input"
	apiKeyHeader   = "DD-API-KEY"
	defaultTimeout = time.Second * 30

	// ContentTypePlain - content is plain text
	contentTypePlain = "text/plain"

	// ContentTypeJSON - content is JSON
	contentTypeJSON = "application/json"

	// Maximum content size per payload: 5MB
	maxContentByteSize = 5*1024*1024 - 2

	// Maximum size for a single log: 256kB
	maxEntryByteSize = 256 * 1024

	// Maximum array size if sending multiple logs in an array: 500 entries
	maxArraySize = 500
)

var (
	// Debug - print out debug log if true
	Debug = false
)

// NewHook - create hook with input
func NewHook(
	host string,
	apiKey string,
	batchTimeout time.Duration,
	maxRetry int,
	minLevel logrus.Level,
	formatter logrus.Formatter,
	options Options,
) *Hook {

	h := &Hook{
		host:      host,
		apiKey:    apiKey,
		maxRetry:  maxRetry,
		minLevel:  minLevel,
		formatter: formatter,
		options:   options,
	}

	if batchTimeout < 5*time.Second {
		batchTimeout = 5 * time.Second
	}
	h.ch = make(chan []byte, 1)
	go h.pile(time.Tick(batchTimeout))
	return h
}

// Levels - implement Hook interface supporting all levels
func (h *Hook) Levels() []logrus.Level {
	return logrus.AllLevels[:h.minLevel+1]
}

// Fire - implement Hook interface fire the entry
func (h *Hook) Fire(entry *logrus.Entry) error {
	line, err := h.formatter.Format(entry)
	if err != nil {
		dbg("Unable to read entry, %v", err)
		return err
	}
	h.ch <- line
	return h.err
}

func (h *Hook) pile(ticker <-chan time.Time) {
	var pile [][]byte
	size := 0
	for {
		select {
		case p := <-h.ch:
			str := string(p)
			if str == "" {
				continue
			}
			if h.isJSON() {
				str = strings.TrimRight(str, "\n")
				str += ","
			} else if !strings.HasSuffix(str, "\n") {
				str += "\n"
			}
			bytes := []byte(str)
			messageSize := len(bytes)
			if size+messageSize >= maxContentByteSize || len(pile) == maxArraySize {
				go h.send(pile)
				pile = make([][]byte, 0, maxArraySize)
				size = 0
			}
			pile = append(pile, bytes)
			size += messageSize
		case <-ticker:
			go h.send(pile)
			pile = make([][]byte, 0, maxArraySize)
			size = 0
		}
	}
}

func (h *Hook) isJSON() bool {
	if _, ok := h.formatter.(*logrus.JSONFormatter); ok {
		return true
	} else if _, ok := h.formatter.(*logrus.TextFormatter); ok {
		return false
	}
	b, err := h.formatter.Format(&logrus.Entry{})
	if err != nil {
		return false
	}
	str := strings.TrimSpace(string(b))
	return strings.HasPrefix(str, "{") && strings.HasSuffix(str, "}")
}

func (h *Hook) send(pile [][]byte) {
	h.m.Lock()
	defer h.m.Unlock()
	if len(pile) == 0 {
		return
	}

	buf := make([]byte, 0)
	for _, line := range pile {
		buf = append(buf, line...)
	}
	if len(buf) == 0 {
		return
	}
	if h.isJSON() {
		if buf[len(buf)-1] == ',' {
			buf = buf[:len(buf)-1]
		}
		buf = append(buf, ']')
		buf = append([]byte{'['}, buf...)
	}

	dbg(string(buf))

	req, err := http.NewRequest("POST", h.datadogURL(), bytes.NewBuffer(buf))
	if err != nil {
		dbg(err.Error())
		return
	}
	header := http.Header{}
	header.Add(apiKeyHeader, h.apiKey)
	if h.isJSON() {
		header.Add("Content-Type", contentTypeJSON)
	} else {
		header.Add("Content-Type", contentTypePlain)
	}
	header.Add("charset", "UTF-8")
	req.Header = header

	i := 0
	for {
		resp, err := http.DefaultClient.Do(req)
		if err == nil && resp.StatusCode < 400 {
			dbg("Success - %d", resp.StatusCode)
			return
		}
		dbg("err  = %v", err)
		dbg("resp = %v", resp)
		i++
		if h.maxRetry < 0 || i >= h.maxRetry {
			dbg("Still failed after %d retries", i)
			return
		}
	}
}

func (h *Hook) datadogURL() string {
	u, err := url.Parse("https://" + h.host)
	if err != nil {
		dbg(err.Error())
		return ""
	}
	u.Path += basePath
	parameters := url.Values{}
	o := h.options
	if o.Source != "" {
		parameters.Add("ddsource", o.Source)
	}
	if o.Service != "" {
		parameters.Add("service", o.Service)
	}
	if o.Hostname != "" {
		parameters.Add("hostname", o.Hostname)
	}
	if o.Tags != nil {
		tags := strings.Join(o.Tags, ",")
		parameters.Add("ddtags", tags)
	}
	u.RawQuery = parameters.Encode()
	return u.String()
}

func dbg(format string, a ...interface{}) {
	if Debug {
		log.Printf(format+"\n", a...)
	}
}
