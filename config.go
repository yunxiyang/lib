package lib

import "time"

func init() {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		panic(err)
	}
	conf = config{
		location: location,
	}
}

var conf config

type config struct {
	location *time.Location
	logger   func(format string, args ...interface{})
}

func SetLocation(location *time.Location) {
	conf.location = location
}

func SetLogger(logger func(string, ...interface{})) {
	conf.logger = logger
}

func logger(format string, args ...interface{}) {
	if conf.logger != nil {
		conf.logger(format, args)
	}
}
