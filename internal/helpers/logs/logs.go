package logs

import (
	"log"
	"os"
)

var IsDebugMode = os.Getenv("DEBUG") != "" && os.Getenv("DEBUG") != "0" && os.Getenv("DEBUG") != "false"

func ShowLogs(msg string) {
	if IsDebugMode {
		log.Println(msg)
	}
}
