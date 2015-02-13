package plugin

// Config Policy
// task > control > default

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"log"
	"time"
)

var (
	// Timeout settings
	// How much time must elapse before a lack of Ping results in a timeout
	PingTimeoutDuration = time.Second * 5
	// How many succesive PingTimeouts must occur to equal a failure.
	PingTimeoutLimit = 3
)

const (
	// List of plugin type
	CollectorPluginType PluginType = iota
	PublisherPluginType
	ProcessorPluginType
)

const (
	// List of plugin response states
	PluginSuccess PluginResponseState = iota
	PluginFailure
)

var (
	// Array matching plugin type enum to a string
	// note: in string represenation we use lower case
	types = [...]string{
		"collector",
		"publisher",
		"processor",
	}
)

type MetricType struct {
	namespace               []string
	lastAdvertisedTimestamp int64
}

func (m *MetricType) Namespace() []string {
	return m.namespace
}

func (m *MetricType) LastAdvertisedTimestamp() int64 {
	return m.lastAdvertisedTimestamp
}

func NewMetricType(ns []string, last int64) *MetricType {
	return &MetricType{
		namespace:               ns,
		lastAdvertisedTimestamp: last,
	}
}

type PluginResponseState int

type PluginType int

// Plugin interface
type Plugin interface {
}

// Returns string for matching enum plugin type
func (p PluginType) String() string {
	return types[p]
}

// Started plugin session state
type SessionState struct {
	*Arg
	Token         string
	ListenAddress string
	LastPing      time.Time
	Logger        *log.Logger
	KillChan      chan int
}

// Arguments passed to startup of Plugin
type Arg struct {
	// Plugin file path to binary
	PluginLogPath string
	// A public key from control used to verify RPC calls - not implemented yet
	ControlPubKey *rsa.PublicKey
	// The listen port requested - optional, defaults to 0 via InitSessionState()
	ListenPort string
	// Whether to run as daemon to exit after sending response
	RunAsDaemon bool
}

// Arguments passed to ping
type PingArgs struct{}

type KillArgs struct {
	Reason string
}

// Response from started plugin
type Response struct {
	Meta          PluginMeta
	ListenAddress string
	Token         string
	Type          PluginType
	// State is a signal from plugin to control that it passed
	// its own loading requirements
	State        PluginResponseState
	ErrorMessage string
}

type ConfigPolicy struct {
}

type PluginMeta struct {
	Name    string
	Version int
}

func (s *SessionState) Ping(arg PingArgs, b *bool) error {
	// For now we return nil. We can return an error if we are shutting
	// down or otherwise in a state we should signal poor health.
	// Reply should contain any context.
	s.LastPing = time.Now()
	s.Logger.Println("Ping received")
	return nil
}

func (s *SessionState) Kill(arg KillArgs, b *bool) error {
	// Right now we have no coordination needed. In the future we should
	// add control to wait on a lock before halting.
	s.Logger.Printf("Kill called by agent, reason: %s\n", arg.Reason)
	go func() {
		time.Sleep(time.Second * 2)
		s.KillChan <- 0
	}()
	return nil
}

func (s *SessionState) generateResponse(r Response) []byte {
	// Add common plugin response properties
	r.ListenAddress = s.ListenAddress
	r.Token = s.Token
	rs, _ := json.Marshal(r)
	return rs
}

func InitSessionState(path, pluginArgsMsg string) (*SessionState, error) {
	pluginArg := new(Arg)
	err := json.Unmarshal([]byte(pluginArgsMsg), pluginArg)
	if err != nil {
		return nil, err
	}

	// If no port was provided we let the OS select a port for us.
	// This is safe as address is returned in the Response and keep
	// alive prevents unattended plugins.
	if pluginArg.ListenPort == "" {
		pluginArg.ListenPort = "0"
	}

	// Generate random token for this session
	rb := make([]byte, 32)
	rand.Read(rb)
	rs := base64.URLEncoding.EncodeToString(rb)

	return &SessionState{Arg: pluginArg, Token: rs, KillChan: make(chan int)}, nil
}

func (s *SessionState) heartbeatWatch(killChan chan int) {
	s.Logger.Println("Heartbeat started")
	count := 0
	for {
		if time.Now().Sub(s.LastPing) >= PingTimeoutDuration {
			count++
			if count >= PingTimeoutLimit {
				s.Logger.Println("Heartbeat timeout expired")
				defer close(killChan)
				return
			}
		} else {
			s.Logger.Println("Heartbeat timeout reset")
			// Reset count
			count = 0
		}
		time.Sleep(PingTimeoutDuration)
	}
}