# AND

A serverless, terminal-only community app for developers. No central
server, no rented cloud infrastructure — every user's computer is a peer,
and the network is the application.

See the project brief for the full vision (decentralized identity, P2P
discovery, gossiping forum, lazy-loaded local sync, certificate-based
serverless moderation, keyboard-only TUI). This README tracks what's
actually implemented so far.

## Status

| Package                    | Status                                                                        |
| -------------------------- | ----------------------------------------------------------------------------- |
| `internal/crypto`          | Done — mnemonic identity + encrypted local storage                            |
| `internal/network`         | Done — libp2p host, DHT + mDNS discovery, GossipSub pubsub                   |
| `internal/storage`         | Done — local SQLite cache (posts, replies, tombstones, TTL/approval columns)  |
| `internal/forum`           | Done — signed post/reply model, P2P propagation, sync protocol, delete msgs   |
| `internal/moderation`      | Done — founder cert chain, moderator certs, ban propagation, rate limiting    |
| `internal/tui`             | Done — login/registration, main menu, forum browser, live chat                |
| `internal/plugin`          | Done — plugin interface + registry + auto-register                            |
| `internal/updater`         | Done — GitHub Releases auto-update via ldflags                                |
| `Eklentiler/admin`         | Done — founder/admin panel: pending posts + approval TUI                      |
| `Eklentiler/moderator`     | Done — moderator panel: pending posts + approval TUI (cert-gated)             |
| `Eklentiler/ozel_chat`     | Done — private direct messages over libp2p streams (`/and/dm/1.0.0`)         |
| `cmd/and`                  | Done — full startup: login → node → forum → plugins → TUI                    |
| `cmd/andmod`               | Done — CLI: pubkey / grant cert / create ban                                  |

## Layout

```
cmd/and/                entry point — login, node bring-up, plugin wiring, TUI
cmd/andmod/             moderation CLI — pubkey, grant, ban
internal/crypto/        decentralized identity: BIP-39 mnemonic → Ed25519 keypair,
                          encrypted at rest (Argon2id + AES-256-GCM)
internal/network/       libp2p transport: host, NAT traversal, Kademlia DHT +
                          mDNS peer discovery, GossipSub pubsub topics
internal/storage/       local SQLite cache — posts, replies, tombstones;
                          TTL + permanent-approval columns; WAL mode
internal/forum/         forum post/reply semantics on top of network+storage:
                          signing, verification, P2P propagation, sync protocol,
                          author-delete, TTL cleanup
internal/moderation/    moderator certificate chain (founder → mod cert → ban msg),
                          libp2p ConnectionGater, ban/revoke/trusted-author propagation
internal/tui/           bubbletea TUI: login, main menu, forum browser, live chat
internal/plugin/        Plugin interface + Registry; Eklentiler/ packages are plugins
internal/updater/       GitHub Releases version check + binary self-update
Eklentiler/admin/       admin panel plugin — pending post list + approval
Eklentiler/moderator/   moderator panel plugin — pending post list + approval
Eklentiler/ozel_chat/   private chat plugin — direct P2P messages via libp2p streams
```

## How identity works

There is no account database. On first run, the TUI's registration screen
asks for a display name (shown to other peers in chat/forum) and a local
passphrase, then AND generates a random 12-word BIP-39 mnemonic and
deterministically derives an Ed25519 keypair from it — that keypair is
simultaneously the user's AND identity *and* their libp2p node's PeerID,
so one seed phrase is enough to both prove who you are and address your
node on the network. The display name is cosmetic only; it carries no
authentication weight, unlike the keypair.

The mnemonic itself is the only true backup; it is never written to disk
in plaintext. Instead, a local passphrase (chosen per device) is run
through Argon2id to derive an AES-256-GCM key that encrypts the name +
mnemonic together into `identity.dat` under `%APPDATA%\and\`. To move to
a new device, re-enter the 12 words there.

## How peer discovery works

Every node runs:

- A Kademlia DHT (`go-libp2p-kad-dht`, dual WAN/LAN mode) — nodes advertise
  themselves under the rendezvous string `and-community/1.0.0` and search
  for others doing the same.
- mDNS — for instant discovery of other AND nodes on the same LAN.
- UPnP port mapping + NAT status reporting + hole punching
  (`libp2p.NATPortMap`, `EnableNATService`, `EnableHolePunching`) for
  reaching peers behind home routers without any port forwarding.

Once peers are found, they're connected to automatically, and a GossipSub
router (`go-libp2p-pubsub`) on top of those connections propagates forum
posts and chat messages (topics `and/forum` and `and/chat`). A direct
libp2p sync protocol (`/and/sync/1.0.0`) catches peers up on history
they missed while offline.

## How moderation works

There is no central ban list. Moderation uses a certificate chain:

1. **Founder key** — the first user to run AND writes their Ed25519 public
   key to `founder.key` in the app data directory. Every other node that
   starts up reads this file and trusts bans signed by keys the founder
   certified.
2. **Moderator certificates** — the founder runs `andmod grant <pubkey>`
   to produce a `bans/mod_<short>.json` file. The file is handed to the
   moderator (e.g. via Özel Chat). The moderator places it in their `bans/`
   folder.
3. **Ban messages** — a moderator runs `andmod ban <peer_id> <reason>
   --cert <cert.json>` to produce a `bans/ban_<short>.json`. When AND
   starts, it publishes all files in `bans/` on the moderation GossipSub
   topic; every peer that receives a valid, unexpired ban refuses further
   connections from the banned peer via libp2p's ConnectionGater interface.

Certificates expire in ≤7 days; bans in ≤30 days. The founder can revoke
a moderator's certificate at any time; revocation propagates immediately.
The founder's own peer cannot be banned.

## Forum post lifecycle

Posts start with a 5-day TTL. If the author requests permanent status
(`permanent_requested=true`), the post appears in the admin/moderator
panel. Approving it via the TUI panel (or by a trusted author auto-approval)
clears the TTL. Unapproved posts expire silently. The author can delete
their own post at any time; a signed `DeleteMsg` is broadcast and stored as
a tombstone so the post can never be re-added via sync.

## Running it

```sh
go run ./cmd/and
```

First run: the TUI asks for a display name and a local passphrase,
generates a new identity, shows its 12-word mnemonic once for you to
write down, and saves an encrypted `identity.dat`. Subsequent runs: just
asks for that passphrase to unlock the existing identity. Either way, it
then brings up a libp2p node, starts DHT/mDNS discovery, joins the
forum/chat/moderation pubsub topics, and drops you into the main menu.

**Custom bootstrap nodes** — place additional multiaddrs (one per line,
`#` comments allowed) in `%APPDATA%\and\bootstrap.txt` to point AND at
AND-specific bootstrap servers without rebuilding the binary.

## Moderation CLI

```sh
# Show your AND public key (needed by founder to issue a cert):
andmod pubkey

# Founder: issue a 7-day moderator certificate:
andmod grant <target_pubkey_hex> --days 7

# Moderator: ban a peer for 30 days:
andmod ban <peer_id> <reason> --cert bans/mod_<short>.json --days 30
```

## Tests

```sh
go test ./...
```

`internal/crypto` covers identity generation/restoration and the
encrypted save/load round trip. `internal/network` covers that the same
mnemonic always yields the same PeerID, and an end-to-end publish/receive
over a directly connected pubsub topic. `internal/tui` covers the
login/registration form's validation and save/unlock round trip, and the
app shell's menu navigation and chat-send logic — all driven directly
against the bubbletea models, without spinning up a real terminal.

## Why these libraries

- **libp2p** (`go-libp2p`, `go-libp2p-kad-dht`, `go-libp2p-pubsub`) — the
  whole point of this project is "no servers", and libp2p is the
  standard toolkit for exactly that: DHT-based discovery, NAT traversal,
  and gossip-based pubsub, all out of the box.
- **Charmbracelet `bubbletea`/`bubbles`/`lipgloss`** — the standard Go
  toolkit for a keyboard-only TUI.
- **`tyler-smith/go-bip39`** — standard, well-audited BIP-39 mnemonic
  implementation.
- **`golang.org/x/crypto/argon2`** — Argon2id for passphrase-based key
  derivation; current recommendation for this use case.
- **`modernc.org/sqlite`** — pure-Go SQLite driver; no CGO required.
