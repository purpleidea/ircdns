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

// This implements the simple client for IRCdns. It works as follows. We first
// check our local cache to see if we have existing data within the appropriate
// TTL. If so we return it immediately. If not, we connect to IRC and send a
// special "ping" message. We wait some time for them to all reply (or we exit
// on first useful reply) and then we update the cache and return our result.
//
// For future versions we'll need each deployed server to have its own public
// and private key pair, with the public parts saved locally for this client to
// know about. When we get a "pong" response, it will have to be signed by the
// key of that server, otherwise we won't trust it. Similarly, our client can
// sign the "ping" message so that invalid messages can be ignored. Obviously
// the signed messages need to include a timestamp so that they aren't replayed.
package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/lrstanley/girc"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// Suffix is required for all our domains. Empty string to disable this.
	// Using a suffix means we'll be able to resolve other things very fast,
	// because we'll never need to ask the IRC server if this is not for us.
	Suffix = ".ircdns"

	// FastReturn returns the result as soon as possible, even if we'd have
	// more useful data to store in our cache for later by waiting a second
	// extra. This is better true if you do "one of" interactive scripts or
	// you are impatient and you don't typically query many host names. The
	// false parameter here is better if you want to batch a big lookup and
	// keep it in the cache for future queries. Useful when you query alot.
	FastReturn = false

	// LongTimeout is the maximum length to wait on the IRC stuff in total.
	LongTimeout = 30 * time.Second // seconds

	// PingTimeout is the maximum amount of time to wait after connection to
	// the IRC channel for servers to reply, before we exit out. When using
	// the FastReturn option, we'll exit earlier if we get a faster reply.
	PingTimeout = 3 * time.Second // seconds

	debug          = false
	quitMsg        = "Bye!"
	defaultChannel = "#ircdns"
)

// set at compile time
var (
	X_program string
	X_version string
	X_server  string = "irc.freenode.net:7000"
	X_channel string = defaultChannel
	X_nick    string = "ircdns"
)

// Data stores a list of IP addresses in string format and the TTL for them.
type Data struct {
	IP  []string
	TTL int64
}

// Entry stores a hostname and an associated mapping of corresponding data.
type Entry struct {
	Name string
	Data
}

// LoadCache loads the cached mapping and data and returns it.
func LoadCache() (map[string]Data, error) {
	// XXX: not implemented
	// XXX: temporary fake data for testing
	m := map[string]Data{
		"example.com": {
			IP:  []string{"192.0.2.13"},
			TTL: 42,
		},
		//"purpleidea.com": {
		//	IP:  []string{"198.51.100.42"},
		//	TTL: 37,
		//},
	}
	return m, nil
}

// SaveCache saves the provided mapping and data.
func SaveCache(input map[string]Data) error {
	// XXX: not implemented
	return nil
}

// FilterCache filters the provided mapping to remove expired data.
func FilterCache(input map[string]Data) map[string]Data {
	m := make(map[string]Data)
	for k, v := range input {
		// XXX: not implemented
		// XXX: if v.TTL is bad {
		//	continue
		//}
		ip := []string{}
		for _, s := range v.IP { // copy
			ip = append(ip, s)
		}
		m[k] = Data{
			IP:  ip,
			TTL: v.TTL,
		}
	}
	return m
}

// Lookup performs the "DNS" lookup and returns any available results.
// TODO: Should we also be returning a list of "aliases" for use?
// NOTE: Returning zero results without error, is a valid return scenario.
func Lookup(queryName string) ([]string, error) {
	logf("Running: %s, version: %s", X_program, X_version)

	q := strings.TrimSuffix(queryName, Suffix)
	if q == "" {
		return nil, fmt.Errorf("empty string")
	}

	logf("Saving cache...")
	m, err := LoadCache()
	if err != nil {
		return nil, errwrap.Wrapf(err, "load cache failed")
	}
	mapping := FilterCache(m) // get rid of expired data

	logf("mapping: %+v", mapping)

	entry, exists := mapping[q]
	if exists && len(entry.IP) > 0 { // TODO: would we want to return an empty list?
		return entry.IP, nil // found in cache
	}
	// nothing in cache, do the lookup

	ctx, cancel := context.WithTimeout(context.Background(), LongTimeout)
	defer cancel() // cancel when we are finished consuming integers

	ch, err := IRC(ctx) // run IRC
	if err != nil {
		return nil, errwrap.Wrapf(err, "not found and irc didn't work")
	}
Loop:
	for {
		select {
		case entry, ok := <-ch:
			if !ok {
				break Loop
			}
			// XXX: validate incoming data
			mapping[entry.Name] = entry.Data
			//logf("Found: %s Looking for: %s", entry.Name, q)
			if FastReturn && entry.Name == q { // found!
				break Loop
			}

			// long timeout is done with ctx
			//case <-time.After(timeout):
			//	break Loop
		}
	}

	logf("Saving cache...")
	SaveCache(mapping) // XXX: error?

	if entry, exists := mapping[q]; exists && len(entry.IP) > 0 { // TODO: would we want to return an empty list?
		return entry.IP, nil // found!
	}

	// TODO: should we save the "not found" result in the cache with a TTL?
	return nil, fmt.Errorf("nothing found in time")
}

// IRC runs the IRC client and returns a stream of results. It might error early
// if it certainly cannot function properly, but otherwise you wait on the chan
// to provide answers. It takes a context which can tell it when to exit. It
// spawns at least one goroutine if it didn't error right away, so make sure you
// cancel it when you're done.
func IRC(ctx context.Context) (chan Entry, error) {
	wg := &sync.WaitGroup{}

	newCtx, cancel := context.WithCancel(ctx)
	//defer cancel() // this is below in the goroutine

	// IRC...
	server, portstr, err := net.SplitHostPort(X_server)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portstr)
	if err != nil {
		return nil, err
	}

	gcfg := girc.Config{
		Server:    server,
		Port:      port,
		Nick:      X_nick,
		User:      X_program,
		Name:      X_program,
		Version:   X_version,
		SSL:       true,
		TLSConfig: &tls.Config{ServerName: server}, //nolint:gosec
		PingDelay: time.Minute,
		// XXX: add a utility function for this and share with main.go
		//HandleNickCollide: func(n string) string {
		//	if strings.HasPrefix(n, X_nick) {
		//		s := strings.TrimPrefix(n, X_nick)
		//		if s == "" {
		//			// TODO: is the rand seeded automatically?
		//			newNick := fmt.Sprintf("%s%d", X_nick, rand.Int63())
		//			newNick = safeNick(newNick)
		//			obj.newNick = newNick
		//			return newNick
		//		}
		//		if _, err := strconv.Atoi(s); err == nil {
		//			// TODO: do we need to check not to return the
		//			// same previous int by accident?
		//			newNick := fmt.Sprintf("%s%d", X_nick, rand.Int63())
		//			newNick = safeNick(newNick)
		//			obj.newNick = newNick
		//			return newNick
		//		}
		//	}
		//	newNick := n + "?" // make up a stupid name
		//	newNick = safeNick(newNick)
		//	obj.newNick = newNick
		//	return newNick
		//},
	}

	conn := girc.New(gcfg)

	// Join a channel once connected.
	conn.Handlers.Add(girc.CONNECTED, func(_ *girc.Client, _ girc.Event) {
		logf("Connected, joining %s...", X_channel)
		conn.Cmd.Join(X_channel)
	})

	// Send a signal on disconnect.
	conn.Handlers.Add(girc.DISCONNECTED, func(_ *girc.Client, _ girc.Event) {
		logf("Disconnected, quitting...")
		// TODO: can this ever get called twice?
		// TODO: should this disconnect scenario be an error?
	})

	conn.Handlers.Add(girc.NOTICE, func(_ *girc.Client, line girc.Event) {
		logf("Notice...")
		logf("Notice: %+v", line)
	})

	conn.Handlers.Add(girc.JOIN, func(_ *girc.Client, line girc.Event) {
		logf("Joined...")
		if line.Source.ID() == conn.GetNick() { // it's me, ignore the rest...
			return
		}
		logf("Join: %s", line.Source.ID())

		// Get the list of nicks after someone else joins. We don't do
		// this when *we* join, because the names list isn't valid yet.
		logf("Nicks: %+v", conn.LookupChannel(X_channel).UserList)
	})

	conn.Handlers.Add(girc.PART, func(_ *girc.Client, line girc.Event) {
		logf("Parted...")

		if line.Source.ID() == conn.GetNick() { // it's me, ignore the rest...
			return
		}
		logf("Part: %s", line.Source.ID())

		// Get the list of nicks after someone else leaves. We don't do
		// this when *we* leave, because who cares.
		logf("Nicks: %+v", conn.LookupChannel(X_channel).UserList)
	})

	conn.Handlers.Add(girc.MODE, func(_ *girc.Client, line girc.Event) {
		logf("Mode...")
	})

	// Get the initial list of names after we join a channel.
	conn.Handlers.Add("353", func(_ *girc.Client, line girc.Event) {
		logf("Names/353...")
		logf("Nicks: %+v", conn.LookupChannel(X_channel).UserList)

		// Now is possibly the first safe time to send a ping message.
		msg := "ping"
		logf("Sending ping")
		conn.Cmd.Message(X_channel, msg)

		// After we send our initial ping, don't wait longer than this!
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer cancel() // kill all of this early
			select {
			case <-time.After(PingTimeout):
				// done waiting for slow IRC servers
				logf("ping timeout reached")

			case <-newCtx.Done(): // unblock
			}
		}()
	})

	ch := make(chan Entry)

	conn.Handlers.Add(girc.PRIVMSG, func(_ *girc.Client, line girc.Event) {
		logf("Privmsg: %s", line.Last())
		//if line.Last() != "ping" {
		//	return
		//}

		// XXX: check the pong is from an authentic source!

		// format from main.go is:
		//msg := fmt.Sprintf("Host: %s, IP: %s", obj.Hostname, obj.ip)
		s := strings.TrimSpace(line.Last())
		a := strings.Split(s, ", ")
		if len(a) != 2 {
			return
		}
		p0 := "Host: "
		p1 := "IP: "
		if !strings.HasPrefix(a[0], p0) || !strings.HasPrefix(a[1], p1) {
			return
		}
		a0 := strings.TrimPrefix(a[0], p0)
		a1 := strings.TrimPrefix(a[1], p1)

		// basic hostnames only for now
		a0 = strings.TrimFunc(a0, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsNumber(r)
		})

		if len(a0) < 1 || len(a1) < len("a.b.c.d") { // need at least some chars
			return
		}

		ip := net.ParseIP(a1)
		if ip == nil {
			return
		}

		// looks reasonable
		entry := Entry{
			Name: a0,
			Data: Data{
				IP: []string{ip.String()}, // only one
			},
		}

		logf("Sending entry: %s\t%s", a0, ip.String())
		select {
		case ch <- entry:
		case <-newCtx.Done(): // unblock
		}
	})

	go func() {
		defer wg.Wait()
		defer close(ch)
		defer cancel() // from the newCtx above

		// Start the client connection process.
		logf("Connecting...")
		abortCh := make(chan struct{})
		go func() {
			// NOTE: this Connect() blocks!
			if err := conn.Connect(); err != nil {
				logf("Connection error: %+v", err.Error())
				close(abortCh)
			}
		}()

		// Wait for disconnect.
		select {
		case <-abortCh:
			// never connected
		case <-newCtx.Done(): // exit early on signal
			// XXX: this *can* run even though we didn't connect...
			logf("Parting...")
			conn.Cmd.Part(X_channel) // no part message needed
			conn.Quit(quitMsg)       // XXX: should we do Part and Close first?
		}
		logf("Done!")
	}()
	return ch, nil
}

func logf(format string, v ...interface{}) {
	if !debug {
		return
	}
	// debugging
	f, err := os.OpenFile("/tmp/nss_ircdns.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return // silent error
	}
	f.WriteString(fmt.Sprintf(format+"\n", v...))
	f.Close()
}
