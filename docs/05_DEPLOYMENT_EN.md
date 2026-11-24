# Deploy to a NAS or home server

[简体中文](05_DEPLOYMENT.md) · [Quick start](../README_EN.md#quick-start)

## Containers

For v0.7.0 on an ARM64 NAS, use the release archive `ani-go-v0.7.0-linux-arm64.tar.gz`. The ARM64 variant of `ghcr.io/xiaoyuerx/ani-go:0.7.0` contains an x86-64 executable and fails with `exec format error` on hosts with native ARM64 execution only. [v0.7.0 release assets](https://github.com/xiaoyueRX/Ani-Go/releases/tag/v0.7.0)

1. In the repository directory, copy `.env.example` to `.env` and enter the downloader address and credentials. Use a container service name or a host address reachable from Ani-Go.
2. Replace `/path/to/your/data` in Compose with an existing NAS share. An empty directory works for a fresh install when the container account can create files and subdirectories there. The default container user is `1000:1000`. If the share already grants write access to another account, set its numeric `PUID` and `PGID` in `.env` before starting. Use `docker run --user UID:GID` for standalone containers.
3. Run `docker compose up -d ani-go`. Visit `http://localhost:20001`, or use the NAS address from another device. On first startup you can set `ANIGO_ADMIN_PASSWORD`. Otherwise the generated administrator password is printed in `docker compose logs ani-go`. [Compose file](../docker-compose.yml) · [Example environment](../.env.example)

| Container path | Purpose |
| --- | --- |
| `/data/db/ani-go.db` | Persistent SQLite database containing accounts, subscriptions, settings and plugin configuration. |
| `/data/backups` | Compose backup directory, configurable with `BACKUP_PATH`. |
| `/data/uploads/avatars`, `/data/ani-go.log` | Uploaded avatars and the application log. The Compose service runs with `/` as its working directory. |
| `/data/media` | Compose places series, films and specials in separate directories beneath this path. |
| `/data/downloads` | One possible download directory. The completed path reported by the downloader must be readable by Ani-Go. |

For a new container created in a NAS interface or with `docker run`, mount `/data`, set the working directory to `/`, and use the database, backup and three media paths from the Compose configuration to reproduce this layout. With only the `/data` mount and its default working directory, the official v0.7.0 image writes the database to `/app/ani-go.db` inside the container. [Compose configuration](../docker-compose.yml)

Leave a subscription’s **Download directory** blank to use the default save settings of Transmission, aria2 or qBittorrent. To override it, enter a literal path understood by the downloader, such as `/data/downloads/series`. Clearing the field restores the default for subsequent downloads. The organizer places completed media in the configured library.

When the download directory is blank, the scheduler still checks the Ani-Go host’s `TV_BASE_PATH` volume against `DISK_MIN_FREE_GB` before submission. An explicit download directory is checked using the existing path or its nearest existing parent on the Ani-Go host. Monitor free space on a remote downloader’s default volume on that machine.

On Synology, fnOS or Unraid, choose one shared parent directory and mount it as `/data` in both containers. This lets Ani-Go resolve the paths reported by the downloader. Hard links require both directories to be on the same filesystem and mount. A copy across filesystems consumes additional disk space. Enable `USE_HARDLINK` to retain the download source for seeding. [Organizer implementation](../internal/organizer/organizer.go)

Before organizing, Ani-Go asks qBittorrent, Transmission or Aria2 which files were selected and have finished downloading. It checks each local video's size against the client. Unselected episodes and unrelated videos in the same folder stay in place. If the client cannot confirm the files, the organizer waits for a later poll.

Move mode stages the complete file on the media library's filesystem, commits the episode record, then removes the download source. A failed database write leaves the source available for retry. After a restart, Ani-Go resumes pending cleanup. It keeps the source if the download file has changed or the library file is missing. To continue seeding, enable hard links. Ani-Go keeps the download source both when linking on the same filesystem and when copying across filesystems. On local Linux ext4, Btrfs, XFS, F2FS or tmpfs volumes that support directory locking, the next organization in the same directory reclaims abandoned Ani-Go staging files. Windows, network shares and other volumes use isolated staging directories. A power loss during a copy on these volumes can leave a hidden staging file. [Organization commit and recovery](../internal/scheduler/organizer_commit.go)

The default series layout uses `Title (Year)/Season 1` or `Title/Season 1` when the year is unknown. Jellyfin can use the parentheses to identify the title and year separately. After an upgrade, if the only existing directory for the same title and year uses the older `TitleYear` layout, later episodes and seasons continue there. You can set a custom TV template in Settings.

### Reuse existing files after a qBittorrent task disappears

If a subscription still shows a download in progress, check that Ani-Go can reach the original qBittorrent instance. If the task was removed from qBittorrent and its files are still on disk:

1. Add the original torrent in a stopped state. Check its info hash, save location and selected files. For a multi-file torrent the save location is usually the parent of the torrent’s top-level directory. Check each release or resolution separately.
2. Run **Force recheck**. A preallocated file can already show its full size on disk. qBittorrent verifies the pieces to determine how much data is missing. Preserve the original file selection for a partial-season download.
3. Start the task after verification. Complete editions can resume seeding, and incomplete editions will download the remaining pieces. Ani-Go uses the original task identifier to reconnect the episode history. Multiple editions can appear in the library while each episode counts once toward season progress.

### Pause library writes when a media volume disconnects

After a volume disconnects, its path can reveal a writable directory on the system disk. To protect a TV library, first verify the correct volume is mounted. Place a regular file named `.anigo-volume-id` in the effective `TV_BASE_PATH` directory and write a single identifier such as `nas-tv`. Set `TV_VOLUME_ID=nas-tv` in the Ani-Go deployment environment and restart the service. Use `MOVIE_VOLUME_ID` and `OVA_VOLUME_ID` for movies and specials, each with a marker in its own library root. Use the same ID for categories sharing the same root. When changing a library path in the web UI, verify the new root and place its marker there first.

Use plain UTF-8 without a BOM, with up to 256 bytes for the identifier and an optional trailing newline. Ani-Go reads the marker before creating directories, publishing files and committing library records. A missing or mismatched marker leaves the download source available for automatic retry when the volume returns. Pending moves also check their volume before removing the source after a restart. Keep the marker on the intended volume and use directories on that volume below the root. Configure an independent nested mount as its own library root. For kernel-enforced protection when a forced detach coincides with a filesystem operation, deny the Ani-Go account write access to the uncovered mount point.

The built-in Bangumi NFO scraper uses the volume verified for each organized media file. If the marker is missing or incorrect when the plugin receives the event or finishes downloading a poster, it skips that sidecar write. Other plugins manage their own file writes.

After remounting a library or restoring a backup, open the subscription details to check all organized files or selected editions. Ani-Go checks the library directory visible to the running instance. With a configured volume ID, a missing or mismatched marker makes the check inconclusive. Without one, a missing result refers to the currently mounted path. Check the mount before taking recovery action. A size match confirms the byte count. The check leaves media files and collection progress intact.

If one edition is missing and the downloader still has its completed task and selected source file, use the restore action on that edition. The download and the existing library parent directory must share a filesystem that supports hard links. Confirm the mounted library before proceeding if you have no volume ID. Ani-Go links the original file at its recorded path and checks that edition again. The torrent, history and manual flags are preserved. For different filesystems, you can copy the verified source to the recorded library path yourself and run the check again.

With aria2, the task must still be available in the current session. Active seeds and retained completed tasks can supply the required member list. When seeding ends immediately, even a normal aria2 restart may discard a completed task. If Ani-Go reports that it cannot verify the original download, check the source file and its recorded library path manually. If you have the original `.torrent`, add it back to aria2 with its original download directory, enable integrity checking and resuming, and turn off overwriting and automatic renaming. Once aria2 has verified the existing file, retry the restore action.

## Backups and restore

Point `BACKUP_PATH` at a writable directory on a persistent volume. Manual and scheduled backups share `BACKUP_KEEP_COUNT`, which defaults to the latest seven archives. The default schedule is midnight. `BACKUP_CRON` accepts five fields, such as `0 3 * * *`, or six fields including seconds. Saving either setting applies it to the running backup scheduler. Deployment environment variables take precedence.

Before upgrading, create and download a backup in Settings. Include episodes if you need their records. The administrator JSON contains settings, subscriptions and optional episodes and may include plugin credentials. Keep it private. The persistent SQLite database retains accounts and the local session signing key. Ani-Go creates the first administrator when the user table is empty. Startup reports an error if it cannot read the database or the stored signing key is damaged. Restore database access or recover the signing key from a working copy of this instance’s SQLite database before restarting. New archives carry a SHA-256 integrity digest checked before restoration. Older archives without a digest remain readable. When restoring across databases, subscriptions with the same title and season retain their source, group and filter settings and their respective episodes. Plugins reload after a successful restore. Restart to apply other startup configuration. [Backup API](API.md)

### Restore a JSON backup on another host

1. Create a backup in Settings on the source instance. Select **Include episode records** to keep the organized history, then download the JSON file.
2. Copy the downloaded JSON directly into the destination instance’s configured `BACKUP_PATH`, preserving its encoding and the filename generated by Ani-Go. If the browser appended ` (1)`, restore the original name. Sign in with the destination administrator account, open Settings, and restore the file from the backup list.
3. Historical episode records keep the source host’s media paths. Transfer the media files separately. Before enabling subscriptions, check the downloader connection, library directories and subscription download directories on the destination. The new download directories apply to future tasks.

On macOS and Linux a torrent member’s filename can contain a literal backslash. Windows treats that character as a directory separator. When a backup identifies such a file, restoration on Windows reports the episode record and rolls back the operation. If you only need settings and subscriptions, create a backup on the source with episode records excluded.

## Upgrade and verify

Check that backups are on a persistent volume. Keep the existing host directory mounted at `/data`. Before recreating an older container, copy its `/app/data/uploads` to `uploads` on the host share if it contains avatars. Retrieve any backups or historical logs still under `/app/data` as well. Run the commands below, then check the health response's `version` and the container logs.

```bash
docker compose pull ani-go
docker compose up -d ani-go
curl -fsS http://localhost:20001/api/health
```

### Trying a release candidate and rolling back

When moving a manually created container to a persistent volume, locate its actual database first. If the file is in the container's writable layer, stop and retain that container. Copy the database and any adjacent `-wal` and `-shm` files together to a private temporary directory. Use SQLite's `.backup` on this copy to create a snapshot and check that `PRAGMA quick_check` returns `ok`. Place the snapshot at `db/ani-go.db` on a new, empty volume, give the container user read and write access, and verify the original login and subscriptions before retiring the old container.

Record the running image digest or keep the working executable, Compose file and environment settings. On the host, use SQLite’s `.backup` command on the active `DB_PATH` to create a separate snapshot. Run `PRAGMA quick_check` on the snapshot and verify it returns `ok`. The SQLite backup interface includes committed WAL transactions. If Compose uses `latest`, pin `image` to the candidate tag or digest before the trial.

To roll back, stop Ani-Go and preserve the upgraded database and any `-wal` and `-shm` sidecars together in a separate directory. Restore the pre-upgrade snapshot to the original `DB_PATH` with its original ownership and permissions. Start the previously recorded image digest or executable. Check the health endpoint’s version, sign in with the existing account, and verify subscription seasons and organized episode paths. The database snapshot restores application records. Check media files and download-client jobs separately.

## Standalone installation

The 1.0 macOS archive runs on Apple Silicon. Linux and Windows each support x86-64 (`amd64`) and ARM64. Intel Mac and 32-bit ARMv7 Linux systems can pin a historical release that provides their archive, such as `v0.7.0`. Older installations may still show an update notification. To move to 1.0, restore the backup on a supported system and transfer your media files separately. [v0.7.0 downloads](https://github.com/xiaoyueRX/Ani-Go/releases/tag/v0.7.0)

On Linux or macOS, the installer fetches the GitHub release archive, verifies it against the release’s `checksums.txt` and installs the executable for the current user. Set `ANIGO_VERSION` on the shell running the installer to select a tag. For example, append `| ANIGO_VERSION=v0.7.0 sh` in place of `| sh` below.

```sh
curl -fsSL https://raw.githubusercontent.com/xiaoyueRX/Ani-Go/main/install.sh | sh
cd "$HOME/.anigo"
"$HOME/.local/bin/anigo"
```

The working directory holds the active `.env` and relative database paths. Stop a running standalone process before upgrading, rerun the installer, then restart it from that same directory. Uninstall with `sh install.sh --uninstall` from a checkout to remove the managed executable and keep your configuration and data. For manual deployment, including Windows, download the platform archive, verify its checksum and launch `./anigo` or `anigo.exe` in a writable working directory. For MCP behind a reverse proxy, check its SSE paths and token configuration. [Releases](https://github.com/xiaoyueRX/Ani-Go/releases) · [MCP connection guide](03_MCP_SPEC_EN.md)
