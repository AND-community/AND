# AND plugins: installing and writing them

Türkçesi: [EKLENTILER.md](EKLENTILER.md)

An AND plugin is a separate program with its own terminal interface. AND starts it from the Plugins
menu, the plugin talks to AND only through an HTTP API inside the computer, and when it exits AND
carries on where it left off. Adding a plugin never requires rebuilding or forking AND.

- [Installing a plugin](#installing-a-plugin)
- [Writing your first plugin](#writing-your-first-plugin)
- [plugin.json](#pluginjson)
- [Runtime environment](#runtime-environment)
- [Permissions](#permissions)
- [SDK packages](#sdk-packages)
- [API endpoints](#api-endpoints)
- [Look and feel](#look-and-feel)
- [Languages](#languages)
- [Testing](#testing)
- [Packaging and distribution](#packaging-and-distribution)
- [Security rules](#security-rules)
- [Languages other than Go](#languages-other-than-go)
- [License](#license)

AND's command names are Turkish; each has an English alias, used below (`and plugin install` is the
same as `and eklenti kur`).

## Installing a plugin

```sh
and plugin install ./weather                          # from a folder
and plugin install weather-1.0.0.zip                  # from a .zip or .tar.gz archive
and plugin install https://example.org/weather-1.0.0.zip --sha256 <hash-published-by-the-author>
and plugin list                                       # bundled and externally installed plugins
and plugin check weather-1.0.0.zip                    # inspect without installing
and plugin remove weather
```

Before installing, AND unpacks and checks the package, then shows you the plugin, its author, the
program's platform, the SHA-256 hashes and the permissions it asks for, and asks you to confirm
(`-y`/`--yes` skips the question):

- `plugin.json` must be at the root or inside the archive's single folder; the plugin's name starts with
  a letter or digit and contains only letters, digits, `_` and `-`.
- There must be a program for this computer: `and-plugin-<name>[.exe]`, or in multi-platform packages
  `and-plugin-<name>_<os>_<arch>[.exe]`. The operating system and architecture are read from the file
  header; **nothing is executed while installing.**
- Paths in the archive cannot point outside the package; links, device files and packages over 512 MB
  are rejected. Downloads use https only.
- A package that uses the name of a plugin bundled with AND is refused (e.g. a fake `konu_ac`).

The plugin is installed into `eklentiler/<name>/` in the data directory (`%APPDATA%\and\eklentiler` on
Windows, `~/.config/and/eklentiler` on Linux, `~/Library/Application Support/and/eklentiler` on macOS),
so AND's own program folder does not need to be writable. `eklentiler/kurulumlar.json` records the
SHA-256 of the program and of `plugin.json`. AND refuses to start a plugin whose files changed after
installation and never treats a folder without a record as a plugin. A plugin installed while AND is
running appears in the menu on the next start.

On first launch AND asks for the permissions the plugin wants; for an externally installed plugin the
consent screen also says that it did not come with AND and that AND's developers have not reviewed it.
Installing the same plugin again from a newer package updates it: granted permissions are kept, and a new
permission is asked for again. AND's automatic updates never touch externally installed plugins.

`and plugin remove` deletes the folder, the record, the on/off state and the granted permissions. Files
the plugin wrote into the data directory itself are kept, because AND cannot know which they are.

> A plugin is a program that runs on your computer under your user account. Permissions limit what it
> can do through AND's API, not what it can do with your files. Install plugins only from sources you
> trust.

## Writing your first plugin

You need Go (the version in AND's `go.mod` or newer) and AND.

```sh
and plugin new weather               # a working plugin project in ./weather
cd weather
go mod tidy                          # fetches AND's SDK and its dependencies
go run . --deneme                    # try it against a fake AND with sample data, without opening AND
go test ./...
go build -o and-plugin-weather.exe . # on Linux/macOS: -o and-plugin-weather
and plugin install .
```

Options: `--module github.com/you/weather` (Go module path; default `example.com/<name>`), `--author`,
`--label` (the name shown in the menu) and `--and <folder>`. The last one writes a `replace` line into
`go.mod` that points at a local copy of AND, for developing against an AND change that is not published
yet.

The generated project:

| File | What it is |
|---|---|
| `plugin.json` | The manifest; AND reads it without running the program |
| `main.go` | `--manifest` (prints the embedded `plugin.json`), `--deneme`, language, theme and the Bubble Tea program |
| `model.go` | The screen: your identity, connected AND nodes and recent chat messages in a full-screen `ui.Page` |
| `model_test.go` | Screen and manifest tests against the fake AND |
| `ceviri.json`, `ceviri.go`, `ceviri_test.go` | Translations of the UI strings into 5 languages and a completeness test |
| `README.md`, `README.tr.md` | Build, install and distribution notes |

The plugin is its own Go module and imports AND under the module path `github.com/lucian95511/and` (the
repository is `github.com/AND-community/AND`; the old path redirects there).

After changing the plugin, rebuild it and run `and plugin install .` again: AND does not start a program
whose hash differs from the installed copy's record.

## plugin.json

```json
{
  "name": "weather",
  "label": "Weather",
  "version": "1.0.0",
  "description": "Weather for the forum",
  "author": "Jane Doe",
  "permissions": ["forum"],
  "min_api": 2
}
```

| Field | Required | Meaning |
|---|---|---|
| `name` | Yes | The plugin's name; the program is called `and-plugin-<name>[.exe]`. Starts with a letter or digit; letters, digits, `_`, `-`; at most 48 characters |
| `label` | No | The name shown in the menu. Empty means the plugin is hidden (like `konu_ac`, which only AND itself starts) |
| `version` | No | Shown in the menu and in `and plugin list` |
| `description` | No | The short description in the menu |
| `author` | No | The author |
| `permissions` | No | Requested permissions ([Permissions](#permissions)); without it only public reads |
| `min_api` | No | The lowest plugin API version it needs; an older AND neither installs nor starts it |

AND strips control characters and terminal escape sequences from the text fields and ignores unknown
permissions.

## Runtime environment

AND starts the plugin in its own folder (next to `plugin.json`), in the same terminal, and leaves the
screen to it until it exits. Environment variables:

| Variable | Meaning | SDK |
|---|---|---|
| `AND_API_ADDR` | AND's API address (`127.0.0.1:<port>`) | `pluginapi.NewClientFromEnv()` |
| `AND_API_TOKEN` | An API key for this launch only; invalid once the plugin exits, never written to a file | `pluginapi.NewClientFromEnv()` |
| `AND_DATA_DIR` | AND's data directory | `pluginapi.DataDir()` |
| `AND_DIL` | The interface language (`tr`, `en`, `de`, `es`, `fr`, `ru`) | `dil.OrtamdanAyarla()` |
| `AND_CATEGORY` | `konu_ac` only: the category selected in the forum | `pluginapi.Category()` |

Prefix the files you write into the data directory with the plugin's name (`weather_settings.json`), and
do not store identities or keys there.

## Permissions

The key a plugin receives carries only the permissions listed in `plugin.json` **and** approved by the
user. Identity, roles, peers, forum posts, personal sites, the general chat and the theme can be read by
every plugin without a permission. A request that needs a permission the plugin lacks gets `403`
(`pluginapi.IsForbidden(err)`).

| Permission | What it allows |
|---|---|
| `dm` | Sending and receiving private messages, their history and read marks (`/api/v1/dm/*`) |
| `file` | Sending files, seeing and answering incoming file requests (`/api/v1/file/*`) |
| `chat` | Writing to the general chat (`POST /api/v1/chat/send`) |
| `forum` | Opening topics and replying (`POST /api/v1/forum/post`, `/forum/reply`) |
| `site` | Publishing and deleting pages of the personal site (`/api/v1/site/publish`, `/site/delete`) |
| `moderation` | Deleting topics, lifting bans and appointing moderators with the founder's or a moderator's authority (`/api/v1/forum/delete`, `/api/v1/mod/*`) |
| `theme` | Changing AND's theme (`POST /api/v1/theme`) |

Ask only for what you use; the user sees every permission on the consent screen. If an update asks for a
new permission, AND asks the user again.

`pluginapi.APIVersion` is the plugin API's version (currently 2). The API only grows in backward
compatible ways; if you depend on a newer endpoint, set `min_api`. `client.APIVersion()` reads the
connected AND's version; an endpoint that an older AND lacks returns `404` (`pluginapi.IsNotFound(err)`).

## SDK packages

These packages can be imported from outside AND's repository:

| Package | Use |
|---|---|
| `github.com/lucian95511/and/pluginapi` | The client (`Client`), JSON types, `Manifest`, permission constants (`IzinForum`…), `APIVersion` |
| `github.com/lucian95511/and/pluginapi/pluginapitest` | The fake AND used in tests and by `--deneme` |
| `github.com/lucian95511/and/ui` | AND's visual language: theme, full-screen page, header, list rows, key hints |
| `github.com/lucian95511/and/pluginui` | `Safe`: cleans network text before it reaches the screen |
| `github.com/lucian95511/and/dil`, `dil/diltest` | Opening in AND's language and testing that translations are complete |
| `github.com/lucian95511/and/protokol` | Network constants, e.g. the forum categories `protokol.Kategoriler` |

Packages under `internal/` are AND's implementation details and cannot be imported from outside.

## API endpoints

Every request carries the header `X-And-Token: <AND_API_TOKEN>`; errors come back as
`{"error": "description"}` (bad request 400, no permission 403, not found 404, rate limit 429). In Go
every endpoint has a `Client` method.

| Method and path | Permission | `Client` |
|---|---|---|
| `GET /api/v1/version` | — | `APIVersion` |
| `GET /api/v1/identity`, `/role`, `/peers` | — | `Identity`, `Role`, `Peers` (peers with `AND: true` are AND nodes) |
| `GET /api/v1/forum/post?id=` | — | `GetPost` |
| `GET /api/v1/forum/all-posts` | — (founder or moderator only) | `AllPosts` |
| `POST /api/v1/forum/post`, `/forum/reply` | `forum` | `CreatePost(category, title, body)`, `CreateReply` |
| `POST /api/v1/forum/delete` | `moderation` | `DeletePost` |
| `GET /api/v1/site/pages` | — | `SitePages` (the user's own pages) |
| `POST /api/v1/site/publish`, `/site/delete` | `site` | `PublishSitePage`, `DeleteSitePage` |
| `GET /api/v1/chat/all`, `/chat/poll` | — | `AllChat`, `PollChat` |
| `POST /api/v1/chat/send` | `chat` | `SendChat` |
| `POST /api/v1/dm/send` | `dm` | `SendDM`, `SendDMQueued` (queued while the recipient is offline) |
| `GET /api/v1/dm/history?from=`, `/dm/poll` | `dm` | `DMHistory`, `DMHistoryFrom`, `PollDM` |
| `GET` `POST /api/v1/dm/read` | `dm` | `DMReadMarks`, `MarkDMRead` |
| `POST /api/v1/file/send`, `/file/consent`; `GET /file/poll`, `/file/consent-poll` | `file` | `SendFile`, `RespondConsent`, `PollFile`, `PollConsent` |
| `GET /api/v1/mod/bans`, `/mod/history`, `/mod/grants`; `POST /mod/unban`, `/mod/grant`, `/mod/revoke` | `moderation` | `AllBans`, `ModHistory`, `GrantedMods`, `Unban`, `GrantModerator`, `RevokeModerator` |
| `GET /api/v1/theme` | — | `GetTheme` |
| `POST /api/v1/theme` | `theme` | `SetTheme` |

The `poll` endpoints are long polls that wait up to 5 seconds and only return what arrives while the
request is open; use `dm/history?from=<last id>` so nothing is missed. Endpoints that publish content are
rate limited locally (`pluginapi/ratelimit.go`); at human pace you never notice.

## Look and feel

Don't hand-pick colours and boxes; use the `ui` package. It is painted with AND's active theme and every
bundled plugin uses it.

```go
if theme, err := client.GetTheme(); err == nil {
    ui.Apply(ui.Theme(theme))
}

func (m model) View() string {
    w := ui.ContentWidth(m.width)
    body := ui.Header(w, []string{"Weather", "Istanbul"}, ui.S.Muted.Render("up to date")) + "\n\n"
    for i, r := range m.rows {
        body += ui.Row(w, i == m.selected, ui.T(r.name), ui.M("  "+r.detail)) + "\n"
    }
    footer := ui.Divider(w) + "\n" + ui.KeyHints(w, ui.H("↑↓", "select"), ui.H("q", "quit"))
    return ui.Page(m.width, m.height, body, footer)
}
```

| Component | Use |
|---|---|
| `ui.Page`, `ui.ContentWidth`, `ui.ContentHeight` | A frame that fills the terminal; body at the top, footer at the bottom |
| `ui.Header`, `ui.Section`, `ui.Divider` | Breadcrumb header, section heading, divider |
| `ui.Row`, `ui.T`, `ui.M`, `ui.C` | A selectable list row; overflowing text is cut with `…` |
| `ui.KeyHints`, `ui.H` | Key hints at the bottom; a hint that doesn't fit moves to the next line |
| `ui.Notice` | A one-line notice (`KindSuccess`, `KindError`, `KindWarning`, `KindPending`) |
| `ui.Tabs`, `ui.KV`, `ui.Bar`, `ui.Spread`, `ui.Block`, `ui.VLine` | Tabs, aligned label–value rows, a fill bar, side-by-side panels |
| `ui.Measure`, `ui.Wrap`, `ui.Truncate` | Reading width (at most 100 columns), wrapping, cutting |
| `ui.S.*`, `ui.P.*` | Ready-made styles and palette colours |

## Languages

Call `dil.OrtamdanAyarla()` in `main`, write strings with `dil.T("…")` and `dil.F("%d konu", n)`, and
put their translations into `ceviri.json`. The source language is Turkish; every string needs `en`, `de`,
`es`, `fr` and `ru`. Plural forms are separated by `|` (Russian has three), so never put a literal `|`
into a string. `diltest.Kapsam` catches missing, unused or format-verb-mismatched translations;
`DIL_ISKELET=missing.json go test` dumps the missing ones into a file.

## Testing

`pluginapitest` runs AND's real API server with in-memory data. Endpoints, input validation, rate limits
and permission checks behave exactly as in AND.

```go
func TestOpensTopic(t *testing.T) {
    and := pluginapitest.Yeni().Ornek()                  // sample peers, topics, messages
    client := and.Test(t, manifestOku().Permissions...)   // only the manifest's permissions
    if err := openTopic(client, "Genel", "Hello"); err != nil {
        t.Fatal(err)
    }
    if len(and.Konular()) != 3 {
        t.Fatal("no topic was opened")
    }
}
```

- `and.Test(t, permissions...)` also sets the environment variables, so code that calls
  `pluginapi.NewClientFromEnv()` connects to the fake AND too.
- Set-up: `KimlikAyarla` (identity), `EsEkle` (peer), `KonuEkle` (topic), `YanitEkle` (reply),
  `SohbetEkle` (chat message), `DMGelsin` (incoming DM), `DosyaGelsin` (incoming file request).
- Results: `Konular`, `SohbetGecmisi`, `GidenDMler`, `SiteSayfalari`, `GidenDosyalar`, `OnayYaniti`,
  `TemaIstekleri`.
- An unknown forum category is rejected, as on the real network.
- `Baslat(dir, permissions...)` works outside tests too; the template's `--deneme` flag uses it.

## Packaging and distribution

Publish one `.zip` or `.tar.gz` with `plugin.json` and the program at the root or inside a single folder.
Name the programs like AND's own release files and one archive serves every platform; installation
picks the one for the user's computer and skips the rest.

```
weather-1.0.0/
  plugin.json
  README.md
  LICENSE
  and-plugin-weather_windows_amd64.exe
  and-plugin-weather_linux_amd64
  and-plugin-weather_darwin_arm64
```

```sh
GOOS=windows GOARCH=amd64 go build -o dist/and-plugin-weather_windows_amd64.exe .
GOOS=linux   GOARCH=amd64 go build -o dist/and-plugin-weather_linux_amd64 .
GOOS=darwin  GOARCH=arm64 go build -o dist/and-plugin-weather_darwin_arm64 .
```

Publish the archive's SHA-256 too (`sha256sum weather-1.0.0.zip`); users check that what they downloaded
is yours with `and plugin install <address> --sha256 <hash>`. GitHub release asset addresses work
directly.

## Security rules

- Pass every piece of network text (names, titles, messages, file names) through `pluginui.Safe` before
  it reaches the screen: terminal escape sequences can write to the clipboard or corrupt the display.
- Decide who sent a message by `DMMsg.VerifiedPeerID` or `DMRecord.PeerID`; a name inside the message or
  in `From` is not verified.
- Save state files by writing a temporary file and renaming it, so an interrupted write cannot corrupt
  the old file.
- Never log `AND_API_TOKEN`, write it to a file or pass it to another process.

## Languages other than Go

The protocol is HTTP and JSON, so a plugin can be written in any language, with two conditions: the
program accepted at installation is a **native executable** (PE on Windows, ELF on Linux, Mach-O on
macOS; not a script), and requests go to `AND_API_ADDR` with the `X-And-Token` header. The JSON fields
match the types in `pluginapi/api.go`; [and-vscode](and-vscode/README.md) uses the same API from
TypeScript.

## License

AND and the SDK packages plugins import are licensed under GNU AGPL-3.0. A plugin that links these
packages into its program is generally considered a work based on AND: when you distribute it, publish it
with its source under AGPL-3.0 (or a compatible license). A program that does not import AND's packages
and only uses the API over HTTP may be considered a separate work; if in doubt, ask a lawyer.

## Plugins in AND's repository

The plugins in this repository (`Eklentiler/`) come with AND and are updated with AND's releases. When
building from source, each is built into its own folder next to its `plugin.json` (see
[README.md](README.md#building-from-source)). [Eklentiler/ornek](Eklentiler/ornek/) is a more detailed
example that also uses AND's internal packages; start a new plugin with `and plugin new`.
