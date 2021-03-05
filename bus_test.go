package ivy

import (
	"os"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestBus_Start(t *testing.T) {
	SetLogger(os.Stderr, logrus.DebugLevel)
	bus := &BusT{
		Name: "Test",
	}
	defer bus.Stop()

	if err := bus.Start("127.255.255.255:2010"); err != nil {
		t.Fatalf("Unable to start ivy bus: %v", err)
	}
}
