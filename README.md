# *IRCdns*: reliable, inexpensive dynamic dns for sysadmins

## About:

`IRCdns` is a low-budget dynamic dns tool for sysadmins. You install a simple
systemd service on any of the remote servers or workstations that you maintain.
The service waits until it receives a "ping" message from a special IRC client.
It then responds with a "pong" message that includes its hostname and ip
address.

To automatically use these results, you install a special plugin into libc's
[name service switch](http://www.gnu.org/software/libc/manual/html_node/Name-Service-Switch.html).
When you make a DNS request, it automatically connects over IRC to retrieve the
results from your servers.

This came about over frustration with constant failures of the `ddclient`
program, and the cost of finding a free dynamic dns service. They're either
expensive or not long-lived. IRC servers have been around much longer, and I
hope they stick around. Donate a few dollars to [me](https://www.patreon.com/purpleidea)
or [Freenode](https://freenode.net/).

## Community:

Come join us in the `IRCdns` community!

| Medium | Link |
|---|---|
| IRC | [#ircdns](https://webchat.freenode.net/?channels=#ircdns) on Freenode |
| GitHub | [purpleidea](https://github.com/sponsors/purpleidea) on GitHub |
| Patreon | [purpleidea](https://www.patreon.com/purpleidea) on Patreon |
| Liberapay | [purpleidea](https://liberapay.com/purpleidea/donate) on Liberapay |

## Status:

IRCdns is a small hack to make my life as a low-budget sysadmin easier. It's
functional for me, however we could make it fancier in many ways. If you'd like
to add features, let me know! It might be fun to sign your messages with your
public GPG key, and have the servers sign theirs so you can avoid spoofing. It's
common for IRC clients to expose their IP addresses, so encryption probably
isn't a high priority.

There's an /etc/nsswitch.conf client that automatically runs the lookups from
IRC for this to work transparently. A daemonized client version (running as a
systemd service) to keep an up-to-date cache would be a fun addition for very
fast performance, at the cost of always having it running. This could also run
on-demand too.

## Dependencies:

You'll need a recent version of `golang` which can be obtained from your distro
packaging. You'll also need the C headers for `libc` which can be obtained by
installing the `glibc-headers` package in the "redhat" family, or the
`libc6-dev` package in the "debian" family.

## Server usage:

You can install it with:

```bash
make && sudo make install
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
sudo make uninstall
```

## Client usage:

You can install it with:

```bash
make client && sudo make client_install
```

Then add:

`ircdns` to the `hosts` line in `/etc/nsswitch.conf`. You should add the entry
before other network backends like `dns` or `mdns` to ensure faster resolution.
If you know of a robust way to automate this step, please let me know.

You can uninstall it with:

```bash
sudo make client_uninstall
```

You'll have to undo any changes to your `/etc/nsswitch.conf` manually.

## Example usage:

If you're running the server somewhere, and it broadcasts a domain named
`example`, then you should be able to resolve the associated ip address by
typing `getent hosts example.ircdns` or via a `ping` with `ping example.ircdns`.

```bash
$ ping -c 1 -W 1 example.com.ircdns
PING example.com.ircdns (192.0.2.13) 56(84) bytes of data.

--- example.com.ircdns ping statistics ---
1 packets transmitted, 0 received, 100% packet loss, time 0ms
```

## Performance:

Performance of a single lookup on a cold-cache is around 10 seconds. This is
mostly due to the performance of the IRC connect process. If you wish to have
high performance results, you can cache heavily, or eventually run a background
or on-demand IRC worker that keeps the cache warm for when you need your result.

## Custom settings:

For custom settings such as the use of a private IRC server or channel,
different IRC nick's, and other features, please [contact](https://purpleidea.com/contact/)
me, and I'll be happy to make you a custom build for a very low price.

## Questions:

Please ask in the [community](#community)!

## Patches:

We'd love to have your patches! Please send them by email, or as a pull request.

## Author:

Created by [James Shubin](https://twitter.com/purpleidea). You can [contact](https://purpleidea.com/contact/)
him for enterprise support.

Happy hacking!
