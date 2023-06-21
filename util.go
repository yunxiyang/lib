package lib

import (
	"bytes"
	"encoding/json"
	"time"
)

func PrettyJSON(b []byte) []byte {
	var prettyJSON bytes.Buffer
	err := json.Indent(&prettyJSON, b, "", "    ")
	if err != nil {
		return b
	}
	return prettyJSON.Bytes()
}

func TimeString(sec int64) string {
	return time.Unix(sec, 0).In(conf.location).Format("2006-01-02 15:04:05")
}
