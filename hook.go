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

// Hook is the struct holding connect information to Datadog backend
type Hook struct {
	Source   string
	Service  string
	Hostname string
	Tags     []string

	host     string
	apiKey   string
	isJSON   bool
	maxRetry int
	buffer   [][]byte
	m        sync.Mutex
	ch       chan []byte
	err      error
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
	isJSON bool,
	maxRetry int,
	batchTimeout time.Duration,
) *Hook {
	h := &Hook{
		host:     host,
		apiKey:   apiKey,
		isJSON:   isJSON,
		maxRetry: maxRetry,
	}

	if batchTimeout <= 0 {
		batchTimeout = defaultTimeout
	}
	h.ch = make(chan []byte, 1)
	go h.pile(time.Tick(batchTimeout))
	return h
}

// Levels - implement Hook interface supporting all levels
func (h *Hook) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.PanicLevel,
		logrus.FatalLevel,
		logrus.ErrorLevel,
		logrus.WarnLevel,
		logrus.InfoLevel,
		logrus.DebugLevel,
	}
}

// Fire - implement Hook interface fire the entry
func (h *Hook) Fire(entry *logrus.Entry) error {
	var fn func(*logrus.Entry) ([]byte, error)
	if h.isJSON {
		fn = (&logrus.JSONFormatter{}).Format
	} else {
		fn = (&logrus.TextFormatter{DisableColors: true}).Format
	}
	line, err := fn(entry)
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
			if h.isJSON {
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
	if h.isJSON {
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
	if h.isJSON {
		header.Add("Content-Type", contentTypeJSON)
	} else {
		header.Add("Content-Type", contentTypePlain)
	}
	header.Add("charset", "UTF-8")
	req.Header = header

	for i := 0; i <= h.maxRetry; i++ {
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			dbg("%v", resp)
			return
		}
		dbg(err.Error())
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
	if h.Source != "" {
		parameters.Add("ddsource", h.Source)
	}
	if h.Service != "" {
		parameters.Add("service", h.Service)
	}
	if h.Hostname != "" {
		parameters.Add("hostname", h.Hostname)
	}
	if h.Tags != nil {
		tags := strings.Join(h.Tags, ",")
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
