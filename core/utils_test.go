package core

import (
	"testing"
)

func TestLog(t *testing.T) {
	//
	SetLogLevel(LogLevelDebug)
	DebugLog("debug")
	InfoLog("info")
	WarnLog("warn")
	ErrorLog("error")
	//
	SetLogLevel(LogLevelInfo)
	DebugLog("debug")
	InfoLog("info")
	WarnLog("warn")
	ErrorLog("error")
	//
	SetLogLevel(LogLevelWarn)
	DebugLog("debug")
	InfoLog("info")
	WarnLog("warn")
	ErrorLog("error")
	//
	SetLogLevel(LogLevelError)
	DebugLog("debug")
	InfoLog("info")
	WarnLog("warn")
	ErrorLog("error")
	//
	SetLogLevel(1)
	ErrorLog("error")
}

func TestReadJSON(t *testing.T) {
	err := ReadJSON("ssdkfcsfk", nil)
	if err == nil {
		t.Error(err)
		return
	}
}
