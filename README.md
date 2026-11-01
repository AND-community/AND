# AND

**English** · [Türkçe](README.tr.md)

**AND** — a serverless, peer-to-peer (P2P) terminal forum community.  
No accounts, no sign-up, no central server. Your identity is a single recovery code.

Website: **[and-community.github.io](https://and-community.github.io/)**

---

## Contents

- [Overview](#overview)
- [Features](#features)
- [Architecture](#architecture)
- [Installation](#installation)
- [Usage](#usage)
- [Moderation](#moderation)
- [Plugin system](#plugin-system)
- [Ecosystem](#ecosystem)
- [Configuration](#configuration)
- [Contributing](#contributing)
- [Security](#security)
- [Privacy and terms of use](#privacy-and-terms-of-use)
- [License](#license)

---

## Overview

AND is a terminal-based forum application that runs without any central server.  
Every node is both a client and a server: messages spread over the [libp2p](https://libp2p.io/) GossipSub protocol, and data is stored only in a local SQLite database.

A user's identity is an Ed25519 key pair derived from 128 bits of entropy (the equivalent of a 12-word BIP-39 mnemonic). The same key pair serves as both the application identity and the libp2p node identity, so a single recovery code brings everything back.

---

## Features

| Feature | Description |
|---------|-------------|
| Serverless | No central infrastructure; the network runs peer to peer |
| BIP-39 identity | 128-bit entropy (equivalent to a 12-word BIP-39 mnemonic, shown on screen as a 32-character recovery code) → Ed25519 key → libp2p identity |
| Ed25519-signed content | Every topic, reply and moderation decision is signed |
| Local SQLite store | WAL mode for high concurrency, schema versioning |
| Forum categories | Python, Rust, Go, Cybersecurity, Artificial Intelligence and more (34 in total) |
| Personal site | Every user can build a personal "site" of independent pages addressed by slug — signed, spread over GossipSub and synced retroactively (edited pages included) |
| Private messages | Direct P2P messages over a libp2p stream (`/and/dm/1.0.0`) |
| File transfer | Resumable chunked transfer (`/and/file/2.0.0`) with a consent dialog |
| Moderation | Flexible bans (1 hour to permanent) and unbans, delegable and revocable moderator certificates, an Admin Panel plugin and an audit history |
| Dynamic plugins | Standalone binaries; add plugins without rebuilding AND |
| Automatic updates | Official release builds update themselves in the background from GitHub Releases, together with the bundled plugins, only if the release's signature is valid; builds made from source never replace themselves |
| Terminal interface | Full-screen TUI built on [Bubble Tea](https://github.com/charmbracelet/bubbletea); AND, its plugins and the AND Search Engine share one design system (`internal/ui`) |
| Themes | 6 built-in themes (Classic, Turquoise, Night Purple, Amber, Forest, Mono) plus theme plugins; the chosen theme also applies to plugins and the VS Code panel |
| Interface languages | Turkish (default), English, German, Spanish, French, Russian; switch instantly in Settings → Language or with `ctrl+l` on the login screen — see [Interface language](#interface-language) |

---

## Architecture

```
cmd/
  and/              — main application entry point
  andmod/           — moderation CLI tool (andmod grant, ban)

Eklentiler/         — plugins ("eklentiler" is Turkish for "plugins")
  admin/            — Admin Panel plugin (standalone binary)
  ozel_chat/        — private messaging + file transfer plugin
  site/             — personal site creation/editing plugin
  ornek/            — guide to writing a new plugin ("örnek" means "example")
  DeltaX/, Pembis/, konu_ac/
                    — separate repositories; clone them here to build them (see Building from source)

internal/
  crypto/           — BIP-39 → Ed25519 identity, AES-GCM encrypted storage
  network/          — libp2p node, GossipSub, DHT discovery, sync protocol
  forum/            — topic/reply management, signature checks, TTL, tombstone sync
  chatlog/          — public P2P chat channel (signed messages, rate limits, sync)
  moderation/       — founder/moderator certificates, flexible bans/unbans, audit
                       history, ConnectionGater
  profile/          — user profiles (bio/status/link/tags), role badges
  site/             — multi-page personal sites (signed page publish/delete, its own sync protocol)
  peersync/         — scheduling of history sync: with which peer, and when (see below)
  ui/               — shared design system: theme/palette, styles, full-screen page frame, components
  storage/          — SQLite WAL, schema versioning
  pluginapi/        — plugin HTTP IPC API server + client library
  pluginmgr/        — and-plugin-* binary discovery and launch
  pluginui/         — ANSI/control-character sanitising for plugin and remote content
  dmmgr/            — libp2p DM stream handler + long-poll proxy for ozel_chat
  filemgr/          — chunked file send/receive; consent mechanism
  githubverify/     — account verification through the GitHub OAuth Device Flow
  bbcode/           — simple BBCode → terminal rendering for forum bodies
  tui/              — Bubble Tea TUI (menu, forum, chat, profiles, moderation actions, settings)
  updater/          — GitHub Releases check and self-update
```

### Data flow — forum topic

```
User → n key (on the forum screen)
                ↓  AND starts and-plugin-konu_ac in the same terminal (AND_CATEGORY)
            konu_ac → POST /api/v1/forum/post → AND → Forum.CreatePost()
                ↓  Ed25519 signature
            GossipSub broadcast (libp2p)
                ↓
        All peers → verify signature → SQLite
```

### Data flow — private message (DM)

```
and-plugin-ozel_chat  →  POST /api/v1/dm/send  →  AND (dmmgr.Broker)
                                                         ↓  libp2p stream
                                                     Target peer

Target peer  →  /and/dm/1.0.0 stream handler  →  dmmgr.Broker.Deliver()
                                                         ↓  encrypted history + long-poll
                                              and-plugin-ozel_chat (displayed)
```

AND stores incoming and outgoing private messages encrypted on this computer (`dm_gecmis.bin`) and
serves them through `GET /api/v1/dm/history`. A message that arrives while Özel Chat (the private
chat plugin) is closed is therefore not lost; it shows up as unread in the conversation list when
the screen opens. Up to 500 messages between people are kept, plus up to 300 machine messages
from plugins (for example DeltaX's `AND-PROJEAG/…` group protocol; see
`protokol.MakineIletisiMi`); machine messages don't count as unread and are hidden in Özel Chat.

### Data flow — file transfer

```
Sender: and-plugin-ozel_chat  →  POST /api/v1/file/send  →  AND (filemgr.Broker)
                                                                      ↓  /and/file/2.0.0 stream
                                                                  Target peer

Target peer  →  /and/file/2.0.0 stream handler  →  wait for consent (30 s)
                       ↓  GET /api/v1/file/consent-poll (long-poll)
               and-plugin-ozel_chat → show dialog → POST /api/v1/file/consent
                       ↓  if accepted
               Chunked transfer → disk → notification via GET /api/v1/file/poll
```

### History sync

GossipSub reaches only the peers that are connected at that moment. AND collects the topics, chat
messages and site pages published while it was offline through separate sync protocols that it
speaks with its peers (`/and/sync/1.0.0`, `/and/chatsync/1.0.0`, `/and/sitesync/1.0.0`).
`internal/peersync` decides when to sync and with whom:

| Situation | Behaviour |
|---|---|
| A new peer connected | Sync after a short wait (so that identify can learn its protocols) |
| The peer doesn't speak the AND protocols | Skipped. Most connected peers are generic IPFS/DHT nodes; opening streams to them would be wasted |
| The peer disconnected and came back | Sync again if 2 minutes have passed since the last successful sync (the content in between arrives) |
| A sync failed | Retried after 1 minute |
| Every 10 minutes | Sync with the 3 connected AND peers that have gone the longest without one; lost gossip messages arrive over time even if the connection never drops |

At most 4 syncs run at once, and two syncs with the same peer never overlap. Even if hundreds of
peers connect at startup, hundreds of streams don't open at once. The top of the main menu shows
how many connected peers are AND nodes and when the last sync happened.

### Resilience

AND treats everything that comes from the network as untrusted and tries to keep a single error
from bringing down the whole node:

| Measure | What it provides |
|---|---|
| Atomic file writes (`internal/dosya`) | `identity.dat`, bans, moderation history, plugin permissions, the theme and DM history are written to a temporary file, flushed to disk and then moved into place. A crash or power cut while writing doesn't corrupt the old file; leftover temporary files are removed at startup |
| Panic guards (`internal/guvenli`) | An unexpected error in code that handles a gossip message or a stream opened by a peer drops only that input (the stack trace goes to `and.log`); the node keeps running. If one of the background loops stops, the interface closes cleanly and shows the error — the terminal is not left broken |
| Fuzz tests | Forum, chat, profile, site and moderation messages and file-transfer frames are exercised with malformed, random and edge-case input, including input validly signed with an attacker's own key (`go test -fuzz`) |
| SQLite settings | `busy_timeout` (5 s), WAL and `synchronous=NORMAL` are applied on every connection; another process using the file at the same time (e.g. `andmod`) doesn't immediately get "database is locked". At shutdown the background jobs stop first, then the database is closed |
| Reconnection | If all peers drop after waking from sleep or a network change, AND reconnects to the bootstrap nodes within 15 s, backing off (up to 5 min) while that fails; once a connection is back it refreshes without waiting for discovery |
| Log limit | `and.log` rotates to `and.log.1` at 5 MB, even while running; a noisy library can't fill the disk |
| One AND per data directory | Only one AND can use a data directory at a time (`and.kilit`, an operating-system lock that is released automatically on a crash). Two ANDs can't write to the same files and overwrite each other's DM history, outbox or ban lists; after an update the restarted version waits for the old one to exit |
| Database backup and recovery | A daily backup of `forum.db` goes into the `yedek/` directory (the newest three, at most 3 days old); an existing database is also backed up before it is upgraded (at most 7 days). Each migration step runs in its own transaction. `PRAGMA quick_check` runs at startup: if the file is corrupt, AND still opens — the corrupt file is moved aside instead of being deleted, the newest good backup is restored, the readable rows are salvaged from the corrupt file and a notice appears on the home screen |
| Unreadable files are not overwritten | A state file that can't be parsed (moderation history, granted moderator certificates, bans, plugin settings, DM history, reply drafts) is moved aside as `<file>.bozuk-<date>` instead of starting empty and being overwritten by the next save |
| Drafts | A forum reply isn't lost on esc, ctrl+c, a send error or AND exiting: it is kept as a per-topic draft (`yanit_taslaklari.json`) and comes back with `r`; only a successful send deletes it. A chat message that couldn't be sent stays in the input box |

---

## Installation

### From a release

Every release on the [Releases](https://github.com/AND-community/AND/releases) page has a ready-to-run
archive for each platform: `AND-<version>-windows-amd64.zip`, `AND-<version>-linux-amd64.tar.gz`,
`AND-<version>-linux-arm64.tar.gz`, `AND-<version>-darwin-amd64.tar.gz` and
`AND-<version>-darwin-arm64.tar.gz`. Extract it anywhere and run `and` (`and.exe` on Windows) in the
extracted folder; the moderation tool `andmod` and all the plugins (in `Eklentiler/`) are already in
place. You can check a download against the release's `SHA256SUMS.txt`, and that file's signature
(`SHA256SUMS.txt.sig`) against the public key in [SECURITY.md](SECURITY.md). A release build keeps
itself and its plugins up to date automatically.

### Requirements

To build from source you need:

- Go 1.26.4 or later (Go 1.21 and later download the toolchain version named in `go.mod` on their own if needed)
- Windows, Linux or macOS (amd64 / arm64)

### Building from source

```bash
git clone https://github.com/AND-community/AND.git
cd AND

# Main binary and moderation tool
go build -o and     ./cmd/and
go build -o andmod  ./cmd/andmod

# Plugins: each is built into its own folder, next to its plugin.json, as
# and-plugin-<name> (<name> is the "name" field of plugin.json)
go build -o Eklentiler/admin/and-plugin-admin          ./Eklentiler/admin
go build -o Eklentiler/ozel_chat/and-plugin-ozel_chat  ./Eklentiler/ozel_chat
go build -o Eklentiler/site/and-plugin-site            ./Eklentiler/site

# DeltaX, Pembiş and Konu Aç live in separate repositories; clone them under Eklentiler/ first
git clone https://github.com/AND-community/DeltaX.git   Eklentiler/DeltaX
git clone https://github.com/AND-community/Pembis.git   Eklentiler/Pembis
git clone https://github.com/AND-community/konu_ac.git  Eklentiler/konu_ac
go build -o Eklentiler/DeltaX/and-plugin-DeltaX    ./Eklentiler/DeltaX
go build -o Eklentiler/Pembis/and-plugin-pembis    ./Eklentiler/Pembis
go build -o Eklentiler/konu_ac/and-plugin-konu_ac  ./Eklentiler/konu_ac
```

On Windows add `.exe` to every binary name. Without Konu Aç ("open a topic") you can't create new
forum topics (the `n` key opens this plugin).

### ldflags — version and repository

Automatic updates run only in builds that embed the release signing key (`UpdateSigningKeyHex`); every
update is applied only after the signature of the release's checksum file has been verified. A build
made from source without the key (e.g. with `calistir.bat`) never checks for updates and never replaces
itself. The release workflow embeds the key from `.github/update-signing-pubkey.hex` together with the
version. To make the same kind of build by hand (if `GitHubRepo` is not given, `AND-community/AND` is
used; `github.com/lucian95511/and` is the Go module path, not the repository name):

```bash
go build \
  -ldflags "-X github.com/lucian95511/and/internal/updater.Version=v1.0.0 \
            -X github.com/lucian95511/and/internal/updater.GitHubRepo=AND-community/AND \
            -X github.com/lucian95511/and/internal/updater.UpdateSigningKeyHex=$(cat .github/update-signing-pubkey.hex)" \
  -o and ./cmd/and
```

### Embedding the founder key in the binary

```bash
# Print the founder public key
./andmod pubkey

# Build the binary with the founder public key
go build \
  -ldflags "-X github.com/lucian95511/and/internal/moderation.FounderPubKeyHex=<64-hex-characters>" \
  -o and ./cmd/and
```

The founder public key is embedded in the binary; the `founder.key` file is created automatically
on the first run.

### Enabling GitHub account verification

The "Verify GitHub account" (v) feature in Settings uses GitHub's OAuth Device Flow and needs a
Client ID:

1. https://github.com/settings/developers → **New OAuth App**.
2. Any value works as the Authorization callback URL (the Device Flow doesn't use it), e.g.
   `https://example.com`.
3. Tick **Enable Device Flow** in the app settings.
4. Take the generated **Client ID** (no Client Secret is needed — the Device Flow is a "public
   client" flow that doesn't need a secret).

```bash
go build \
  -ldflags "-X github.com/lucian95511/and/internal/githubverify.ClientID=<client-id>" \
  -o and ./cmd/and
```

If no Client ID is set, the feature doesn't fail silently — pressing "v" shows a clear error that
no Client ID is configured.

---

## Usage

### Directory layout (runtime)

Plugins live in the `Eklentiler/` folder next to `and`, each in its own folder:

```
/any/directory/
  and[.exe]
  andmod[.exe]
  Eklentiler/
    admin/      plugin.json  and-plugin-admin[.exe]
    konu_ac/    plugin.json  and-plugin-konu_ac[.exe]
    ozel_chat/  plugin.json  and-plugin-ozel_chat[.exe]
    …
```

At startup AND looks in the `Eklentiler/*/` folders next to its own binary for a `plugin.json`
and an `and-plugin-<name>` binary (`<name>` is the `name` field of plugin.json). An
`and-plugin-*` binary placed directly next to `and` is loaded only if a manifest with the same
name sits beside it (`and-plugin-x.json`, see [Writing a new plugin](#writing-a-new-plugin)); a
file without a manifest is never executed, for security reasons.

### First run

```bash
./and
```

A new identity is created on the first start:

1. Pick a display name (cosmetic only; it doesn't affect signatures)
2. Enter a strong password to protect your identity
3. **Write the recovery code down somewhere safe** (`XXXXXXXX-XXXXXXXX-XXXXXXXX-XXXXXXXX`, 32 characters) — it is the only way to get your identity back if you forget your password or move to another computer. You are asked for the code once more to confirm that you wrote it down

On later starts you only need your password.  
If you forgot your password or moved to another computer, press `ctrl+r` on the login screen and
type your recovery code (dashes and spaces don't matter; the 12-word codes of older versions work
too): your identity is regenerated and saved with a new password. The old identity file whose
password was forgotten is not deleted but moved aside as `identity.dat.eski-<date>`. The code has
no checksum, so a mistyped character produces a different identity; AND remembers the last
identity opened on this device (`kimlik_ozeti.json`) and warns you if the code produces a
different one.  
Identity file: `%APPDATA%\and\identity.dat` (Linux: `~/.config/and/identity.dat`, macOS:
`~/Library/Application Support/and/identity.dat`)

Once the identity is ready, AND asks you once to accept a summary of the
[Privacy Notice](GIZLILIK.md) and the [Terms of Use](KULLANIM_KOSULLARI.md) (both documents are in
Turkish; the in-app summaries follow the interface language). The acceptance is written to
`kurallar_onayi.json` in the data directory; when the texts change in a way that affects users
(their version number goes up), you are asked again on the next start. If the rules don't fit on a
small terminal, they can be accepted only after scrolling to the end of the text.

### Command line

Without flags AND opens the full-screen interface. These flags answer without opening the
interface and follow the selected interface language:

| Flag | Prints |
|---|---|
| `-v`, `--version` | Version, Go version and platform, copyright notice, license and source-code address |
| `--lisans`, `--license` | The full text of AND's license (GNU AGPL-3.0) |
| `--ucuncu-taraf`, `--third-party` | The license notices of the third-party components built into AND |
| `-h`, `--help` | The list of flags |

Include the output of `and --version` when you report a bug.

On the login screen `ctrl+l` (or `F2`) changes the interface language; the choice is saved and
used on later starts too (see [Interface language](#interface-language)). `ctrl+r` restores an
identity from its recovery code (see [First run](#first-run)).

### Main menu

AND uses the whole terminal. The top of the main menu shows the emblem, your identity and a few
counts. Below it, the menu tree is on the left and the details of the selected item are on the
right: recent topics and the category breakdown for Forum, recent messages and the most active
writers for Chat, and the description, version and status for plugins. Very wide terminals get a
second column next to the details. On narrow or short terminals the detail panel and the large
emblem hide themselves.

```
╭─  AND  ─────────────────────────────────────────────────────────────────── Classic theme · v0.1.0 ─╮
│                                                                                                    │
│   █████╗ ███╗   ██╗██████╗                                                                         │
│  ██╔══██╗████╗  ██║██╔══██╗      umut                                           Forum  142 topics  │
│  ███████║██╔██╗ ██║██║  ██║      12D3KooW9r…dWngf13xEF                       Chat  1,204 messages  │
│  ██╔══██║██║╚██╗██║██║  ██║      ● 4 peers connected                       Plugins  4 / 5 enabled  │
│  ██║  ██║██║ ╚████║██████╔╝                                                                        │
│  ╚═╝  ╚═╝╚═╝  ╚═══╝╚═════╝                                                                         │
│                                                                                                    │
│  ────────────────────────────────────────────────────────────────────────────────────────────────  │
│                                                                                                    │
│  ▾ Community                   │  Forum                                            34 categories   │
│  ▌ ├── Forum                   │  Topics sorted into categories. A topic spreads to peers as       │
│    └── Chat                    │  soon as it is published and deletes itself after 5 days.         │
│                                │                                                                   │
│  ▾ Plugins                     │  Recent topics                                                    │
│    ├── Admin Panel             │                                                                   │
│    ├── DeltaX                  │  ▸ How do I connect from behind a NAT with libp2p?                │
│    └── Website                 │    Go  ·  zeynep  ·  2h ago  ·  3 replies                         │
│                                │                                                                   │
│  ▾ System                      │  ▸ Common mistakes with Ed25519 signatures                        │
│    ├── Settings                │    Cybersecurity  ·  mehmet  ·  5h ago                            │
│    ├── About AND               │                                                                   │
│    └── Quit                    │                                                                   │
│  ────────────────────────────────────────────────────────────────────────────────────────────────  │
│  ↑↓ navigate · enter open · q quit                                                                 │
│                                                                                                    │
╰────────────────────────────────────────────────────────────────────────────────────────────────────╯
```

> Installed plugin binaries (`and-plugin-*`) are discovered automatically and added to the
> "Plugins" group. `space` on a selected plugin turns it off or on; a disabled plugin is marked
> as such. Plugins with `Label: ""` (e.g. `konu_ac`) don't appear in the menu; they are opened
> directly from the forum. The "Admin Panel" entry is visible only to the founder or to holders of
> a valid (unrevoked) moderator certificate — no other user ever sees it.

### Settings screen

It has four tabs; switch with `←`/`→` (or `tab`), and `esc` returns to the main menu.

| Tab | Contents |
|---|---|
| Info | Version, platform, data directory; identity (name, public key, role, activity), recovery code (`m` shows/hides it) and profile fields (`b` bio, `s` status, `w` link, `t` tags, `v` GitHub verification) |
| Plugins | Name, version, status and description of the installed plugins; `space` turns them on/off |
| Themes | Built-in themes (with colour swatches) and theme plugins; `enter` applies the selected theme immediately |
| Language | Interface language; `enter` applies and saves the selected language immediately |

### Appearance and themes

All of AND's screens and its plugins use the same visual language. Every screen sits inside a frame
that fills the whole terminal. The top edge of the frame shows the `AND` badge on the left and the
active theme and version on the right; each screen starts with a breadcrumb such as
"Forum › Go › Topic" and ends with a hint line in the form "key description". The selected row is
painted across the full width with a `▌` accent bar on its left; notices are one-line `✓`/`✗`
messages. Long text such as a topic body wraps at 100 columns at most so that lines stay easy to
read. This language is defined in the `internal/ui` package (palette, styles, full-screen page frame
and components); plugins use the same package. The AND Search Engine is a separate Go module, so it
repeats the same palette on its own.

Built-in themes:

| Theme | Accent | Use |
|---|---|---|
| Classic (default) | blue-purple | The look AND has had since day one |
| Turquoise | turquoise | A cooler, greenish blue |
| Night Purple | purple | Warmer, mostly purple |
| Amber | amber | Yellow/orange accent, reminiscent of old phosphor screens |
| Forest | green | Green accent, earth-toned names |
| Mono | white/grey | No colour; for screenshots and presentations |

In every theme the selected-row text and the secondary text reach a contrast of at least 4.5:1 on a
dark terminal background (WCAG AA). The floor is 3:1 only for the accent colour, which is used for
bold headings, keys, the emblem and the frame. This is checked by the `internal/ui` tests.

The chosen theme is written to `theme.json` in the data directory and reaches plugins through
`GET /api/v1/theme`: every plugin reads it at startup and paints itself in the same colours, and the
Live Panel of [and-vscode](and-vscode/README.md) takes its accent from it too. Theme plugins (e.g.
`and-plugin-pembis`) can apply their own colours with `POST /api/v1/theme`. Older versions
mistakenly saved the blue-purple default under the name "Turkuaz" (Turquoise); such a `theme.json`
is migrated to the Classic theme automatically.

### Interface language

AND's interface is available in six languages: Turkish (default), English, German, Spanish, French
and Russian. You pick the language in Settings → Language or with `ctrl+l` (`F2`) on the login
screen; the screen is redrawn in the new language immediately and the choice is written to
`dil.json` in the data directory. The `AND_DIL` environment variable (e.g. `AND_DIL=en ./and`)
overrides the saved choice. Dates (month names), thousands separators and number-dependent
singular/plural forms (including the three Russian forms) follow the selected language as well.

Only the interface is translated: forum topics, category values and chat messages are always the
same on the network. For example, a category travels over the network as "Yapay Zeka" and is shown
as "Artificial Intelligence" in the English interface — peers using different languages see the
same topics.

The translations live in the `dil` package ("dil" is Turkish for "language"; it uses only the
standard library) and in the `dil/ceviriler/*.json` files. The Turkish source text is the key; a
text without a translation is shown in Turkish. In code, texts are marked with `dil.T("…")`,
formatted ones with `dil.F("%d konu", n)` and package-level tables with `dil.K("…")`. The `dil`
tests check that every marked text in the sources, every forum category and the names and
descriptions of the bundled plugins are translated into all languages, and that the translations
carry the same format verbs (`%s`, `%d`); the `internal/tui` tests also draw the screens in every
language and verify that the full-screen layout holds and that no Turkish text is left over. A
skeleton of the missing translations of a new text is produced like this:

```bash
DIL_ISKELET=eksik.json go test ./dil -run Kapsam
```

All bundled plugins (DeltaX, Özel Chat, Site, Admin Panel, konu_ac, Pembiş, Example Plugin) open in
AND's language: AND passes the active language to the plugin it starts through `AND_DIL`, and the
plugin calls `dil.OrtamdanAyarla()` at startup. Each plugin keeps its own catalog in its own
folder:

| File | Purpose |
|---|---|
| `ceviri.json` | Translations of the plugin's own texts (same format as `dil/ceviriler`). Texts that are already in AND's catalog ("vazgeç", "Peer ID" …) are not repeated; they come from there |
| `ceviri.go` | Embeds the catalog with `//go:embed` and merges it with `dil.Ekle` in `init` |
| `ceviri_test.go` | `diltest.Kapsam(t, "ceviri.json", ceviriVerisi, diltest.Tara(t, "."))` — catches missing or unused translations and differences in format verbs and in leading/trailing spaces |

The plugins' screen tests also draw every screen in every language with `diltest.HerDilde` and
`diltest.YeniDenetci` and look for untranslated text. Values that are sent over the network
(categories, branch names, protocol fields) and menu identifiers compared in `switch` statements
stay Turkish; they are marked with `dil.K` and translated with `dil.T` only for display. Skeleton of
a new plugin's missing translations:

```bash
DIL_ISKELET=eksik.json go test ./Eklentiler/<name> -run CeviriKapsami
```

### Keyboard shortcuts — forum

| Key | Action |
|-----|--------|
| `↑` / `↓` or `j` / `k` | select a topic |
| `enter` | open the topic |
| `n` | new topic (opens the konu_ac plugin) |
| `tab` | change category |
| `esc` | back to the main menu |

### Personal site

Creating and editing your own multi-page personal site is done with the **Site** plugin (see
[Eklentiler/site/README.md](Eklentiler/site/README.md), in Turkish — keyboard shortcuts,
validation rules). There you see and edit only YOUR OWN pages; another user's site is shown
read-only under the "Site" section of their profile screen (see the profile shortcuts below).

A page can be at most 8000 bytes, and an author can have at most 20 pages.

### Keyboard shortcuts — profile screen (forum + chat)

Go to the author's name in a forum topic, or press `ctrl+p` in chat to open the profile of the
selected message's author:

| Key | Action |
|-----|--------|
| `b` | ban (visible only to the founder/moderators with a valid certificate) |
| `u` | lift the ban (visible only if the target is already banned and you have the right) |
| `esc` | back to the previous screen |

### Finding your Peer ID

Your own Peer ID is shown in the identity section of the main menu.  
You can share this ID with another user to exchange private messages.

### Custom bootstrap nodes

To connect quickly to known AND nodes, add multiaddr lines to `%APPDATA%\and\bootstrap.txt`:

```
/ip4/1.2.3.4/tcp/4001/p2p/12D3KooW...
# this line is a comment — ignored
/dns4/and-node.example.com/tcp/4001/p2p/12D3KooW...
```

---

## Moderation

AND's moderation system is founder-centred and certificate-based; it can be managed both inside
the app (TUI) and with the `andmod` CLI.

There is no topic approval in the forum: a topic spreads over the network the moment it is written,
and every topic, whoever wrote it, is deleted automatically **5 days** after it was created. The
role of the moderators and the founder is to delete an unwanted topic early without waiting for it
to expire (see [Admin Panel](#built-in-plugins)) and to ban peers.

### Roles

| Role | Rights | How to get it |
|-----|-------|-------------------|
| **Regular user** | Write topics, reply, chat | Create an identity |
| **Moderator** | Delete any topic, ban/unban peers | Receive a certificate from the founder (valid for at most 7 days; can also be permanent) |
| **Founder** | All of the above + appoint/revoke moderators, can also lift their own ban | Own the public key embedded in the binary |

If a moderator certificate is revoked (`revoke`), it is invalid from that moment on even if it has
not expired yet — the moderator immediately loses the right both to ban and to lift bans.

### Ban duration

The duration of a ban is flexible: **1 hour**, **1 day**, **7 days**, **30 days** or **lifetime
(permanent)**. A ban is not split into separate "scopes" (chat/forum) — it is a single
network-wide ban that blocks both the banned peer's forum/chat messages and its direct connections
to other nodes (at the transport layer, with a `ConnectionGater`).

If a ban was a mistake or has to be lifted before it expires, **any** valid moderator or the
founder can lift it (unban), independently of the moderator who issued it.

### Inside the app (TUI)

Ban/unban actions can be done right inside the app, without going to the CLI:

- **From the profile screen** (go to the profile of a topic's author in the forum, or press
  `ctrl+p` in chat to open the profile of a message's author): if you are the founder or a
  moderator with a valid certificate, you can ban with `b`, and lift the ban with `u` if the target
  is already banned. The profile also shows the target's current ban status (⛔ Banned, remaining
  time/lifetime, reason).
- **The Admin Panel plugin** (the "Admin Panel" entry on the main menu, visible only to the
  founder/moderators; see [Eklentiler/admin/README.md](Eklentiler/admin/README.md), in Turkish):
  besides the Topics tab, three tabs —
  - *Bans* — shows all active bans in a single list; `u` lifts them from there too.
  - *History* — a permanent audit log: who banned/unbanned whom, when and why (expired/lifted
    entries included; nothing is ever deleted).
  - *Moderators* (founder only) — `g` appoints a new moderator (with duration options), `r`
    revokes an existing moderator's certificate.

### andmod CLI

For moderation while the app isn't running, or from automation/scripts:

```bash
# Print your own public key
./andmod pubkey

# Grant moderator rights (7 days — the upper limit)
./andmod grant <target_pubkey_hex> --days 7

# Grant moderator rights without expiry
./andmod grant <target_pubkey_hex> --permanent

# Ban a peer as a moderator (flexible duration)
./andmod ban <peer_id> "reason" --cert bans/mod_<short>.json --duration 1h
./andmod ban <peer_id> "reason" --cert bans/mod_<short>.json --permanent

# Ban directly with the founder identity (no certificate needed)
./andmod ban <peer_id> "reason" --as-founder --duration 24h

# Lift a ban (with a moderator certificate or as the founder)
./andmod unban <peer_id> --cert bans/mod_<short>.json
./andmod unban <peer_id> --as-founder

# Revoke a moderator's rights (founder only)
./andmod revoke <moderator_pubkey_hex>
```

The generated `.json` files are written to `%APPDATA%\and\bans\`. AND scans this directory and
publishes it to the network only **once at startup** (after 5 seconds) — if you run a CLI command
while the app is already running, it has no effect until AND is restarted. For actions that must
take effect immediately, use the Admin Panel plugin in the TUI.

---

## Plugin system

AND plugins are **standalone executable binaries**. Each plugin is compiled as a separate Go
program and can be distributed independently of AND.

### How it works

1. At startup AND looks in the `Eklentiler/*/` folders next to its own binary for a `plugin.json` and an `and-plugin-<name>` binary. An `and-plugin-*` binary placed directly next to `and` is loaded only if an `and-plugin-<name>.json` manifest sits beside it.
2. The first time the user opens a plugin, AND asks for the permissions listed in its `plugin.json`.
3. AND starts the plugin binary in the same terminal with these environment variables:

   | Variable | Description |
   |----------|-------------|
   | `AND_API_ADDR` | AND's localhost HTTP API address (e.g. `127.0.0.1:48291`) |
   | `AND_API_TOKEN` | The plugin's API key; generated per plugin at startup and never written to a file |
   | `AND_DATA_DIR` | AND's data directory (for draft files) |
   | `AND_CATEGORY` | (konu_ac only) The category selected in the forum |
   | `AND_DIL` | The active interface language (`tr`, `en`, `de`, `es`, `fr`, `ru`; see [Interface language](#interface-language)) |

4. The plugin performs identity, forum, DM and file operations by sending HTTP requests to AND at `AND_API_ADDR`.
5. When the plugin's TUI exits, AND takes control back.

> `konu_ac` has no menu entry (its `Label` is empty); the `n` key in the forum starts it with the
> selected category.

### Standalone external tools (e.g. the VS Code extension)

The `AND_API_ADDR`/`AND_API_TOKEN` mechanism above only reaches plugin subprocesses that AND
**starts itself** — an independently started process (e.g. an editor extension) never sees these
environment variables, because the random port and the random token exist only in memory for each
run and are never written to any file.

To solve this safely there is a second listener that is **opt-in and off by default**, enabled with
the `AND_ENABLE_EXTERNAL_API=1` environment variable
(`internal/pluginapi/externalpipe_windows.go` / `externalpipe_unix.go`):

- **Windows:** a **named pipe** whose security descriptor grants access only to the user who opened
  the listener (`D:P(A;;GA;;;<user SID>)`; `OW` if the SID can't be read).
- **Linux/macOS:** a **Unix domain socket** at `AND_DATA_DIR/api.sock` with `0600` permissions.

This second listener has **no** bearer token — access is restricted not by a secret but directly by
the operating system (same user only); the pipe/socket name is derived from `AND_DATA_DIR` and is
not secret (see `and-vscode/src/live/liveAddr.ts`, which computes it independently with the same
formula). If `AND_ENABLE_EXTERNAL_API` is not set, this listener is never opened and nothing changes.
Example use: the live Forum/DM/Chat/Network/Site panel in [and-vscode](and-vscode/README.md), and
editing and publishing personal site pages in the VS Code editor.

Endpoints that publish or send content (forum, site, chat, DM, file) are also subject to a local
rate limit (`internal/pluginapi/ratelimit.go`) — ANY plugin on the token listener, or any local
process on the external API, could otherwise flood the network quickly with the node's own valid
signature; without this layer such a bug or abuse would spread over the network unnoticed. It never
affects AND's own TUI; it only kicks in for automated or faulty use that exceeds dozens of calls per
minute, and returns `429 Too Many Requests`.

### Built-in plugins

| Binary | Menu label | Description |
|--------|-------------|----------|
| `and-plugin-admin` | Admin Panel | Topics/Bans/History tabs (founder/moderator) + Moderators tab (founder only) — see [Eklentiler/admin/README.md](Eklentiler/admin/README.md) |
| `and-plugin-konu_ac` | *(hidden)* | New-topic form; opened from the forum with the `n` key (separate repository: [AND-community/konu_ac](https://github.com/AND-community/konu_ac)) |
| `and-plugin-ozel_chat` | Özel Chat (private chat) | Direct P2P messaging: conversation list with unread counts, session history, sending files and consenting to incoming ones (see [Eklentiler/ozel_chat/README.md](Eklentiler/ozel_chat/README.md)) |
| `and-plugin-pembis` | Pembiş Theme | Paints the interface in shades of pink (separate repository: [AND-community/Pembis](https://github.com/AND-community/Pembis)) |
| `and-plugin-DeltaX` | DeltaX | Private groups + their own version control system; end-to-end encrypted P2P sync (separate repository: [AND-community/DeltaX](https://github.com/AND-community/DeltaX)) |
| `and-plugin-site` | Website | Create/edit/delete your own multi-page personal site — no role restriction (see [Eklentiler/site/README.md](Eklentiler/site/README.md)) |

The READMEs of the plugins inside this repository are in Turkish.

### Writing a new plugin

See [Eklentiler/ornek/README.md](Eklentiler/ornek/README.md) (in Turkish) for a detailed template
and the API reference.

Short version:

```bash
# 1. Create the Eklentiler/<name>/ directory and write main.go
mkdir Eklentiler/myplugin

# 2. Build it and put it into the AND directory
go build -o and-plugin-myplugin ./Eklentiler/myplugin

# 3. Generate the manifest JSON
./and-plugin-myplugin --manifest > and-plugin-myplugin.json

# AND discovers the plugin automatically on its next start
```

Plugins run only with the permissions they request: the `permissions` field in `plugin.json` (e.g.
`["dm", "file"]`) is shown to the user on first launch; reading public content needs no permission.
For the list of permissions and endpoints see
[Eklentiler/ornek/README.md](Eklentiler/ornek/README.md#i̇zinler).

To have your plugin open in the same language as AND, call `dil.OrtamdanAyarla()` in `main`, write
texts with `dil.T`/`dil.F` and put their translations into `ceviri.json` (see
[Interface language](#interface-language)).

---

## Ecosystem

**[AND Search Engine](https://github.com/AND-community/and-arama-motoru)** ("arama motoru" is Turkish for "search engine") is a separate system in its own repository; to build it, clone it next to AND. It is an independent peer that joins AND's P2P network with its own identity and makes forum/chat content searchable in a local SQLite FTS5 index. It doesn't use AND's internal packages, doesn't run AND as a subprocess and keeps working while AND is closed — two separate programs that speak **the same network** (the same GossipSub topics, the same Ed25519 signature format, the same sync protocols). Topic names and protocol IDs come from the `protokol/` package that both programs use (the search engine takes it from the AND checkout next to it); sample messages produced with AND's real code (`protokol/testdata/`) are verified in the search engine's tests, so if the message format changes on one side, the tests break.

What shows that they belong to the same family:
- The same ANSI 256-colour palette (accent/error/success/muted tones — see `internal/ui`) and the same "AND" block-letter logo.
- Exactly the same identity derivation scheme (BIP-39 mnemonic → Ed25519), but the two programs have COMPLETELY independent identities.
- The signature payload formats and GossipSub topic names of the forum/chat/personal-site protocols are identical — which is why the Search Engine can verify and index the content produced by AND. This is tested on every test run with the samples in `protokol/testdata/`.

For details, see the search engine's own README: [AND-community/and-arama-motoru](https://github.com/AND-community/and-arama-motoru).

---

## Configuration

All data is stored under `%APPDATA%\and\` (Windows), `~/.config/and/` (Linux) or
`~/Library/Application Support/and/` (macOS). Which data is visible to whom is described in
[GIZLILIK.md](GIZLILIK.md) (the privacy notice, in Turkish).

| File / directory | Description |
|---|---|
| `identity.dat` | AES-256-GCM encrypted Ed25519 identity (password protected) |
| `founder.key` | The founder's Ed25519 public key (derived from the one embedded in the binary) |
| `forum.db` | SQLite forum + chat database (WAL mode) |
| `bans/` | Moderation decisions (ban/unban/revoke/certificate JSON files — written by `andmod` and the Moderators tab of the TUI) |
| `active_bans.json` | Snapshot of the bans currently in force (restored on restart) |
| `active_revocations.json` | List of revoked moderator certificates |
| `mod_history.json` | Moderation audit history (the "History" tab of the Admin Panel plugin) |
| `granted_mods.json` | Local ledger of the moderator certificates issued by this node (if it is the founder) |
| `mod_rates.json` | Hourly ban/unban/revoke rate-limit counters per moderator |
| `dosyalar/` | Received files |
| `bootstrap.txt` | Extra bootstrap node multiaddrs (optional) |
| `taslaklar_<category>.json` | Topic drafts (konu_ac plugin, local — gitignored) |
| `theme.json` | Selected theme (written by Settings → Themes or by a theme plugin; Classic is used if it is missing) |
| `dil.json` | Selected interface language (Settings → Language or `ctrl+l` on the login screen; Turkish if it is missing; the `AND_DIL` environment variable overrides it) |
| `kurallar_onayi.json` | Which version of the privacy summary and the community rules was accepted, and when |
| `dm_gecmis.bin` | Private message history, outbox and read receipts; AES-256-GCM encrypted with a key derived from the identity (`AND_DM_GECMISI=0` keeps them in memory only) |
| `yedek/` | Daily (at most 3 days) and pre-upgrade (at most 7 days) backups of `forum.db` |
| `yanit_taslaklari.json` | Unsent forum replies (per topic; deleted when sent or after 30 days) |
| `kimlik_ozeti.json` | Public key and display name of the last identity opened on this device — used to warn you when a recovery code is mistyped |
| `and.kilit` | The operating-system lock held by the AND that uses the data directory (keeps a second AND from opening) |
| `*.bozuk-<date>`, `*.okunamadi-<date>`, `identity.dat.eski-<date>` | Files moved aside because they couldn't be read or were replaced; AND never deletes them |
| `and.log` | Warnings and errors written by libraries (libp2p, mDNS) while the interface is open; they go here instead of the terminal so that they don't break the full-screen interface; stack traces of unexpected errors are here too. Rotates to `and.log.1` above 5 MB (also while running) |

> Never share the contents of `identity.dat`, `founder.key` and `bans/` with anyone, and never add
> them to git.

---

## Contributing

Read [CONTRIBUTING.md](CONTRIBUTING.md) (in Turkish) to contribute.

---

## Security

Read [SECURITY.md](SECURITY.md) (in Turkish) to report a security vulnerability.  
Don't report vulnerabilities as public issues; get in touch directly by e-mail.

---

## Privacy and terms of use

- [GIZLILIK.md](GIZLILIK.md) (privacy notice, in Turkish): which data AND keeps where, who can see
  it, which peers it connects to on the network and how the data is deleted. AND collects no
  telemetry.
- [KULLANIM_KOSULLARI.md](KULLANIM_KOSULLARI.md) (terms of use, in Turkish): compliance with the
  law, community rules, liability and moderation.

Summaries of both are in the Privacy and Rules tabs of the **About AND** screen in the app and are
presented for acceptance on the first start. When a change affects users, the version number in the
documents and `kurallarSurumu` in `internal/tui/hakkinda.go` are raised together; everyone accepts
the new version on their next start.

---

## License

Copyright (C) 2026 AND contributors.

AND is licensed under version 3 of the GNU Affero General Public License (AGPL-3.0); the full text
is in the [LICENSE](LICENSE) file. The software is provided without any warranty, to the extent
permitted by law. If you modify AND and make it available to others over a network, section 13 of
the license requires you to offer them the source code of your modified version too.

The license notices are also inside the program: the **About AND** screen on the main menu,
`and --lisans` and `and --ucuncu-taraf`.

### Third-party components

The binaries of AND and its plugins include more than 140 open-source components (libp2p, Bubble
Tea, modernc SQLite, the Go standard library …). All components are under licenses that can be
distributed together with AGPL-3.0: MIT, BSD-2/3-Clause, Apache-2.0, ISC and MPL-2.0. The version
and full license text of each component are in
[internal/lisans/ucuncu_taraf.txt](internal/lisans/ucuncu_taraf.txt); in releases this file is
published next to the binaries as `THIRD_PARTY_NOTICES.txt`.

The notice is generated from `go.mod`:

```bash
go generate ./internal/lisans
```

The `internal/lisans` tests check that the notice carries the same modules and versions as `go.mod`
and that every dependency's license is compatible with AGPL-3.0; CI additionally verifies that the
generated files are up to date.
