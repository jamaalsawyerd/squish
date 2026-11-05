<div align="center">

# squish

**Squish your images down to size, in your terminal.**

A friendly app for compressing photos and graphics, with live size estimates and
a tiny hydraulic press.

![squish compressing a folder of photos](docs/demo.gif)

</div>

## Why squish?

- **Small files that still look right.** One Squeeze setting works the same
  across formats, tuned on real camera photos.
- **See it before you commit.** Watch the estimated size update as you change
  settings, then press `o` to open the full result in your own image viewer.
- **Safe by default.** Originals are never touched, a file that wouldn't get
  smaller is left alone, and photos stay the right way up.
- **Fast.** 22 camera originals took 13 seconds on a 16-thread machine.
- **One small binary.** No ImageMagick, no libvips, no setup.
- **Actually fun.** There's a press, confetti, achievements, and your savings
  measured in floppy disks.

## Install

Download a binary for Linux, macOS or Windows from the
[releases page](https://github.com/jamaalsawyerd/squish/releases), or build it
with Go 1.26 or newer:

```sh
go install -tags nodynamic github.com/jamaalsawyerd/squish/cmd/squish@latest
```

## Use it

Open the app and pick your files:

```sh
squish
```

Or start with a folder or some files already loaded:

```sh
squish ~/Pictures/holiday
```

You can also drag files into the terminal window at any time.

### Without the app

Add `--no-tui` for scripts. squish also does this by itself when its output is
piped.

```sh
# Resize for the web, keeping the originals
squish --no-tui --max-size 2000 --out web/ photos/

# Let squish choose the best format for each image
squish --no-tui --format auto *.png

# Squeeze as hard as possible
squish --no-tui --format avif --preset small photos/
```

Run `squish --help` to see every option.

## Keys

| Screen | Keys |
|---|---|
| **Pick** | `space` select, `a` select all, `enter` open, `backspace` up a folder, `g` go to a path, `.` hidden files, `tab` next step |
| **Tune** | `↑` `↓` choose a setting, `←` `→` change it, `o` open the result in your image viewer, `[` `]` previous or next file, `enter` squish! |
| **Squishing** | `esc` stop after the files in progress |
| **Results** | `o` open the saved file in your image viewer, `r` try new settings, `n` new batch |

Press `o` on the Tune screen and squish compresses the whole image with your
current settings, saves it to a temporary file and opens it in your default image
viewer. Nothing is written beside your originals, and temporary files are deleted
when you quit. Over SSH, or with no viewer to open, squish shows the file's path
instead.

## How much does it save?

These are 22 real camera originals from NASA's image library, 230 MB in total,
straight from Canon, Nikon and Hasselblad cameras:

| Setting | Output | Smaller by | Time |
|---|---:|---:|---:|
| JPEG, High | 63 MB | 72% | 14 s |
| **JPEG, Balanced** (the default) | **41 MB** | **82%** | **13 s** |
| JPEG, Small | 30 MB | 87% | 12 s |
| WebP, Balanced | 60 MB | 74% | 29 s |
| AVIF, Balanced | 27 MB | 88% | 134 s |

Your results will vary. Sharp, detailed or low-light photos save less, and images
that were already compressed save much less.

## Formats

squish reads JPEG, PNG, WebP, AVIF and GIF, and writes JPEG, PNG, WebP and AVIF.

| Format setting | What happens |
|---|---|
| **Same as input** | Each file keeps its format. This is the default. |
| **Auto** | Photos become JPEG. Screenshots and graphics use lossless WebP or PNG, whichever is smaller. |
| **JPEG** | Great for photos, and works everywhere. Transparency is replaced by a background color. |
| **PNG** | Uses a palette when that still looks good, and keeps full color when it doesn't. |
| **WebP** | Lossless for graphics, lossy for photos. |
| **AVIF** | The smallest files, but much slower, and needs over 1 GB of memory per photo. |

## Good to know

- Output goes beside each file as `name-squished.jpg`, or into the folder you
  choose. Existing files are never overwritten.
- `--in-place` replaces originals, but only when the new file is smaller.
- EXIF data is removed by default, which also removes any GPS location. Use
  `--keep-metadata` to keep it.
- Color profiles are kept where the format allows, except in AVIF, where squish
  warns you that colors may shift.
- Animated images are skipped for now, rather than being flattened to one frame.
- PNG has no lossless optimizer yet, so lossless savings are small. For bigger
  savings, try Format: Auto.
- Achievements and lifetime totals are saved in your user config folder, in
  `squish/stats.json`. Nothing leaves your machine.

## Development

```sh
make build    # Build bin/squish
make test     # Run the tests with the race detector
make lint     # Run golangci-lint
make samples  # Download sample photos into samples/
make demo     # Re-record docs/demo.gif (needs VHS)
```

[docs/codecs.md](docs/codecs.md) explains why squish uses the libraries it does,
with the measurements behind each choice.

## Thanks

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea),
[Lip Gloss](https://github.com/charmbracelet/lipgloss),
[Bubbles](https://github.com/charmbracelet/bubbles),
[Harmonica](https://github.com/charmbracelet/harmonica) and
[Fang](https://github.com/charmbracelet/fang) from Charm. Image encoding and
decoding comes from the excellent libraries by
[gen2brain](https://github.com/gen2brain).

The photos in the demo are public domain images from the
[NASA Image and Video Library](https://images.nasa.gov/). This project is not
affiliated with or endorsed by NASA.

## License

[MIT](LICENSE)
