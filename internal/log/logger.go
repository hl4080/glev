package log

import (
	"github.com/sirupsen/logrus"
	"os"
)

var Logger *logrus.Logger = nil

func init() {
	Logger = logrus.New()
	Logger.SetFormatter(&logrus.TextFormatter{
		ForceColors		: true,
		FullTimestamp	: true,})
	Logger.Out = os.Stdout
}
