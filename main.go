// IRCdns
// Copyright (C) 2019-2020+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/chyeh/pubip"
	irc "github.com/fluffle/goirc/client"
	ircstate "github.com/fluffle/goirc/state"
	mgmtutil "github.com/purpleidea/mgmt/util"
)

const (
	ipErrorRetry   = 15 * time.Second
	ipPollInterval = 60 * time.Second
	quitMsg        = "Bye!"
	defaultChannel = "#ircdns"
	maxNickLength  = 16
)

// set at compile time
var (
	X_program string
	X_version string
	X_server  string = "irc.freenode.net:7000"
	X_channel string = defaultChannel
	X_nick    string = "ircdns"
	X_me      string = "purpleidea"
)

type Main struct {
	// Program name to share with the IRC server.
	Program string
	// Version string to share with the IRC server.
	Version string
	// Server is an fqdn:port combination of the IRC server to join.
	Server string
	// Channel is the name of the IRC channel to join. Omit the # to message
	// a user directly.
	Channel string
	// Nick is the IRC nick name to use.
	Nick string
	// Me is the IRC nick that we reveal information to.
	Me string
	// Hostname to use when sending a message. Determined automatically if
	// absent.
	Hostname string

	ip      net.IP
	ipevent chan struct{} // ip changed events

	conn  *irc.Conn
	state ircstate.Tracker

	// names is the current list of names in the room.
	names   []string
	newNick string

	wg   *sync.WaitGroup
	exit *mgmtutil.EasyExit // exit signal
}

func (obj *Main) Init() error {
	if obj.Program == "" {
		return fmt.Errorf("missing Program name")
	}
	if obj.Version == "" {
		return fmt.Errorf("missing Version string")
	}
	if obj.Server == "" {
		return fmt.Errorf("missing Server name as <fqdn>:<port>")
	}
	if obj.Channel == "" {
		return fmt.Errorf("missing Channel name")
	}
	if obj.Nick == "" {
		return fmt.Errorf("missing Nick value")
	}
	if len(obj.Nick) > maxNickLength {
		return fmt.Errorf("the Nick is too long, max length of %d", maxNickLength)
	}
	obj.newNick = obj.Nick // store initial value
	if obj.Me == "" {
		return fmt.Errorf("missing Me value")
	}

	var err error
	if obj.Hostname == "" {
		if obj.Hostname, err = os.Hostname(); err != nil {
			return err
		}
	}
	fmt.Println(fmt.Sprintf("This is: %s, version: %s", obj.Program, obj.Version))
	fmt.Println("Copyright (C) 2019-2020+ James Shubin and the project contributors")
	fmt.Println("Written by James Shubin <james@shubin.ca> and the project contributors")

	log.Printf("Server: %s", obj.Server)
	log.Printf("Channel: %s", obj.Channel)
	log.Printf("Nick: %s", obj.Nick)
	log.Printf("Me: %s", obj.Me)

	obj.ipevent = make(chan struct{})

	obj.wg = &sync.WaitGroup{}
	obj.exit = mgmtutil.NewEasyExit()

	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		// must have buffer for max number of signals
		signals := make(chan os.Signal, 1+1) // 1 * ^C + 1 * SIGTERM
		signal.Notify(signals, os.Interrupt) // catch ^C
		//signal.Notify(signals, os.Kill) // catch signals
		signal.Notify(signals, syscall.SIGTERM)
		var count uint8
		for {
			select {
			case sig := <-signals: // any signal will do
				if sig != os.Interrupt {
					log.Printf("interrupted by signal")
					obj.exit.Done(fmt.Errorf("killed by %v", sig)) // trigger exit
					return
				}

				switch count {
				case 0:
					log.Printf("interrupted by ^C")
					obj.exit.Done(nil) // trigger exit
				//case 1:
				//	log.Printf("interrupted by ^C (fast pause)")
				case 2:
					//	log.Printf("interrupted by ^C (hard interrupt)")
				}
				count++

			case <-obj.exit.Signal():
				return
			}
		}
	}()

	return nil
}

func (obj *Main) Run() error {
	host, _, err := net.SplitHostPort(obj.Server)
	if err != nil {
		return err
	}

	// Get public IP address.
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()

		for {
			// XXX: this asks some random service...
			log.Printf("Getting IP...")
			ip, err := pubip.Get()
			// FIXME: consider explicitly using our own instead...
			//ip, err := GetIPBy("checkip.dyndns.org")
			if err != nil {
				log.Printf("Could not get IP: %+v", err)
				// XXX: use exponential backoff instead...
				select {
				case <-time.After(ipErrorRetry):
				case <-obj.exit.Signal():
					return
				}
				continue
			}

			// On first run or changed IP, send an event...
			if obj.ip == nil || !obj.ip.Equal(ip) {
				obj.ip = ip // store
				log.Printf("IP address changed to: %s", obj.ip)
				select {
				case obj.ipevent <- struct{}{}:
					// send
				case <-obj.exit.Signal():
					return
				}
			}

			// TODO: Use an event-based method instead of polling.
			select {
			case <-time.After(ipPollInterval):
			case <-obj.exit.Signal():
				return
			}
		}
	}()

	// Block until we get an initial IP address...
	select {
	case <-obj.ipevent:
		// discard
	case <-obj.exit.Signal():
		return nil
	}

	// IRC...
	cfg := irc.NewConfig(obj.Nick)
	cfg.Me.Name = obj.Program  // hide the name of the IRC lib
	cfg.Me.Ident = obj.Program // hide the name of the IRC lib
	cfg.Version = obj.Version  // hide the name of the IRC lib
	cfg.QuitMessage = quitMsg
	cfg.SSL = true
	cfg.SSLConfig = &tls.Config{ServerName: host} // no :port at the end
	cfg.Server = obj.Server
	cfg.NewNick = func(n string) string {
		if strings.HasPrefix(n, obj.Nick) {
			s := strings.TrimPrefix(n, obj.Nick)
			if s == "" {
				// TODO: is the rand seeded automatically?
				newNick := fmt.Sprintf("%s%d", obj.Nick, rand.Int63())
				newNick = safeNick(newNick)
				obj.newNick = newNick
				return newNick
			}
			if _, err := strconv.Atoi(s); err == nil {
				// TODO: do we need to check not to return the
				// same previous int by accident?
				newNick := fmt.Sprintf("%s%d", obj.Nick, rand.Int63())
				newNick = safeNick(newNick)
				obj.newNick = newNick
				return newNick
			}
		}
		newNick := n + "?" // make up a stupid name
		newNick = safeNick(newNick)
		obj.newNick = newNick
		return newNick
	}
	obj.conn = irc.Client(cfg)
	disconnect := false // did we get a disconnect?
	obj.conn.EnableStateTracking()
	obj.state = obj.conn.StateTracker()

	// NOTE: Possible event list that we might want to use.
	// "JOIN"
	// "KICK"
	// "MODE"
	// "NICK"
	// "PART"
	// "QUIT"
	// "TOPIC"
	// These numbers can be seen here: https://modern.ircdocs.horse/
	// "311": whois reply
	// "324": mode reply
	// "332": topic reply
	// "352": who reply
	// "353": names reply
	// "671": whois reply (nick connected via SSL)

	// Join a channel once connected.
	obj.conn.HandleFunc(irc.CONNECTED, func(_ *irc.Conn, _ *irc.Line) {
		log.Printf("Connected, joining %s...", obj.Channel)
		obj.conn.Join(obj.Channel)
	})

	// Send a signal on disconnect.
	obj.conn.HandleFunc(irc.DISCONNECTED, func(_ *irc.Conn, _ *irc.Line) {
		disconnect = true
		log.Printf("Disconnected, quitting...")
		// TODO: can this ever get called twice?
		// TODO: should this disconnect scenario be an error?
		obj.exit.Done(nil) // trigger exit
	})

	obj.conn.HandleFunc(irc.NOTICE, func(_ *irc.Conn, line *irc.Line) {
		log.Printf("Notice...")
		//log.Printf("Notice: %+v", line)
	})

	obj.conn.HandleFunc(irc.JOIN, func(_ *irc.Conn, line *irc.Line) {
		log.Printf("Joined...")

		if line.Nick == obj.getNick() { // it's me, ignore the rest...
			return
		}
		log.Printf("Join: %s", line.Nick)

		// Get the list of nicks after someone else joins. We don't do
		// this when *we* join, because the names list isn't valid yet.
		log.Printf("Nicks: %+v", obj.getNames())

		obj.sendMsg() // consider sending if someone joined
	})

	obj.conn.HandleFunc(irc.PART, func(_ *irc.Conn, line *irc.Line) {
		log.Printf("Parted...")

		if line.Nick == obj.getNick() { // it's me, ignore the rest...
			return
		}
		log.Printf("Part: %s", line.Nick)

		// Get the list of nicks after someone else leaves. We don't do
		// this when *we* leave, because who cares.
		log.Printf("Nicks: %+v", obj.getNames())
	})

	obj.conn.HandleFunc(irc.MODE, func(_ *irc.Conn, line *irc.Line) {
		log.Printf("Mode...")
	})

	// Get the initial list of names after we join a channel.
	obj.conn.HandleFunc("353", func(_ *irc.Conn, line *irc.Line) {
		log.Printf("Names/353...")
		log.Printf("Nicks: %+v", obj.getNames())

		obj.sendMsg() // consider sending once we're in the channel
	})

	// Start the client connection process.
	log.Printf("Connecting...")
	if err := obj.conn.Connect(); err != nil {
		log.Printf("Connection error: %+v", err.Error())
	}

	// Send messages to channel.
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()

		for {
			select {
			case <-obj.ipevent:
				obj.sendMsg() // send message

			case <-obj.exit.Signal():
				return
			}
		}
	}()

	// Wait for disconnect
	select {
	case <-obj.exit.Signal(): // exit early on exit signal
		if !disconnect {
			log.Printf("Parting...")
			obj.conn.Part(obj.Channel) // no part message needed
		}
		obj.conn.Quit(quitMsg) // XXX: should we do Part and Close first?
	}
	obj.wg.Wait()
	log.Printf("Done!")

	return nil
}

// getNick returns the actual Nick being used. This is different than obj.Nick
// if we got renamed because of a 433 event meaning the name is already in use.
func (obj *Main) getNick() string {
	n := obj.state.Me() // ask the state tracker

	// If we're using a changed nick, *and* the currently reported nick is
	// the original nick, then the state tracker must be slow or broken, so
	// use our own locally tracked state in this case.
	if obj.newNick != obj.Nick && n.Nick == obj.Nick {
		return obj.newNick
	}

	return n.Nick
}

// getNames gets a list of names in the channel and also updates our main cache.
func (obj *Main) getNames() []string {
	c := obj.state.GetChannel(obj.Channel)
	n := []string{}
	if c == nil { // this can happen if we pick a nick longer than 16 chars!
		log.Printf("Bug:getNames:c is nil!")
		//obj.conn.Raw("NAMES" + " " + obj.Channel) // ask again?
		return n
	}
	for k := range c.Nicks {
		n = append(n, k)
	}
	sort.Strings(n)
	obj.names = n // store it
	return n
}

func (obj *Main) sendMsg() {
	if !mgmtutil.StrInList(obj.Me, obj.getNames()) {
		return // skip sending if i'm not in the channel
	}
	msg := fmt.Sprintf("Host: %s, IP: %s", obj.Hostname, obj.ip)
	log.Printf("Sending message: %s", msg)
	obj.conn.Privmsg(obj.Channel, msg)
}

func safeNick(nick string) string {
	if len(nick) > 16 {
		return nick[0:maxNickLength]
	}
	return nick
}

func main() {
	channel := defaultChannel
	if X_channel != "" {
		channel = X_channel
	}
	m := &Main{
		Program: X_program,
		Version: X_version,
		Server:  X_server,
		Channel: channel,
		Nick:    X_nick,
		Me:      X_me,
	}
	if err := m.Init(); err != nil {
		log.Printf("Error during Init: %+v", err)
		os.Exit(1)
		return
	}

	if err := m.Run(); err != nil {
		log.Printf("Error during Run: %+v", err)
		os.Exit(1)
	}

	os.Exit(0)
}
