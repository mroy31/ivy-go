package main

import (
	"os"
	"time"

	"github.com/mroy31/ivy-go"
	"github.com/sirupsen/logrus"
)

var (
	busId = "127.255.255.255:2010"
)

func OnAppConnect(agent ivy.IvyApplication) {
	logrus.Infoln("App " + agent.Name + " is connected")
}

func OnAppDie(agent ivy.IvyApplication) {
	logrus.Infoln("App " + agent.Name + " is disconnected")
}

func OnMsg(agent ivy.IvyApplication, params []string) {
	logrus.Infof("Receive new msg: '%s'", params[0])
}

func main() {
	ivy.SetLogger(os.Stderr, logrus.InfoLevel)
	if err := ivy.IvyInit("ivy-go-example", "Ready", 0, OnAppConnect, OnAppDie); err != nil {
		logrus.Fatalf("Unable to init ivy bus: %v", err)
	}

	ivy.IvyBindMsg(OnMsg, "(.*)")

	if err := ivy.IvyStart(busId); err != nil {
		logrus.Fatalf("Unable to start ivy bus: %v", err)
	}
	time.Sleep(1 * time.Second)

	if err := ivy.IvySendMsg("Ivy example message"); err != nil {
		logrus.Fatalf("Unable to send ivy message: %v", err)
	}

	list, _ := ivy.IvyGetApplicationList()
	for _, agent := range list {
		logrus.Infof("Application connected: %d - %s", agent.ID, agent.Name)

		subscriptions, _ := ivy.IvyGetApplicationMessages(agent.ID)
		for _, sub := range subscriptions {
			logrus.Infof("\tSubscription: %d - %s", sub.ID, sub.Regexp)
		}
	}

	err := ivy.IvyMainLoop()
	if err != nil {
		logrus.Fatalf("IvyMainLoop ends with an error: %v", err)
	}
}
