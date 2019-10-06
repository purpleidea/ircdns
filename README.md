# *IRCdns*: reliable, inexpensive dynamic dns for sysadmins

## About:

`IRCdns` is a low-budget dynamic dns tool for sysadmins. You install a simple
systemd service on any of the remote servers or workstations that you maintain.
This service will periodically ping an IRC channel with its hostname and public
IP address. This should make it easy for you to connect to it over SSH in
emergencies. This came about over frustration with constant failures of the
`ddclient` program, and the cost of finding a free dynamic dns service. They're
either expensive or not long-lived. IRC servers have been around much longer,
and I hope they stick around. Donate a few dollars to [me](https://www.patreon.com/purpleidea)
or [Freenode](https://freenode.net/).

## Community:

Come join us in the `ircDNS` community!

| Medium | Link |
|---|---|
| IRC | [#ircdns](https://webchat.freenode.net/?channels=#ircdns) on Freenode |
| Patreon | [purpleidea](https://www.patreon.com/purpleidea) on Patreon |
| Liberapay | [purpleidea](https://liberapay.com/purpleidea/donate) on Liberapay |

## Status:

IRCdns is a small hack to make my life as a low-budget sysadmin easier. It's
functional for me, however we could make it fancier in many ways. If you'd like
to add features, let me know! An `IRCdns` client that automatically plugged into
your /etc/nsswitch.conf and kept track of your hosts might be a fun addition. It
might also be fun to encrypt your messages with your public GPG key, however
it's common for IRC clients to expose their IP addresses anyways.

## Usage:

You can install it with:

```bash
make install
```

If you have problems use:

```bash
sudo systemctl status ircdns.service
```

And to follow the logs use:

```bash
sudo journalctl -fu ircdns.service
```

You can uninstall it with:

```bash
make uninstall
```

## Questions:

Please ask in the [community](#community)!

## Patches:

We'd love to have your patches! Please send them by email, or as a pull request.

## Author:

Created by [James Shubin](https://twitter.com/purpleidea). You can [contact](https://purpleidea.com/contact/)
him for enterprise support.

Happy hacking!
