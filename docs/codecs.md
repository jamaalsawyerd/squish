# Codec choices

Why squish uses the libraries it does. Numbers come from benchmarks run in
November 2026 on a 16-thread Linux machine with 27 GB of memory (WSL2, Go 1.26.5).

## Stack

| Job | Library | Why |
|---|---|---|
| Decode JPEG | `gen2brain/jpegn` | About 2x faster than `image/jpeg`, applies EXIF rotation |
| Encode JPEG | `gen2brain/jpegli` | The smallest JPEG output at good quality |
| PNG | `gen2brain/pngn` | Keeps ICC and other metadata, which `image/png` drops |
| Palette reduction | `ericpauley/go-quantize` | Median cut, used without dithering |
| WebP | `gen2brain/vpx/webp` | About 4x faster than the WASM encoder, within a few percent on size |
| AVIF | `gen2brain/avif` | Smallest output, but slow and memory hungry, so opt-in |

All of it builds with `CGO_ENABLED=0 -tags nodynamic`. Release binaries are 11 to
13 MB on all six targets.

## Real camera photos

The main test set is 22 originals from NASA's image library, 230 MB in total,
from Canon, Nikon and Hasselblad cameras at 15 to 61 MP.

| Setting | Output | Smaller by | Time | Peak memory |
|---|---:|---:|---:|---:|
| JPEG, High | 63 MB | 72% | 14 s | |
| JPEG, Balanced | 41 MB | 82% | 13 s | 11.4 GB |
| JPEG, Small | 30 MB | 87% | 12 s | |
| WebP, Balanced | 60 MB | 74% | 29 s | |
| AVIF, Balanced | 27 MB | 88% | 134 s | 12.5 GB |

A separate batch of 600 photos under 2 MP took 14 s, peaking at 1.1 GB.

Peak memory depends on the machine, because the engine budgets half of total
memory. These runs had a 13.5 GB budget, so a smaller machine will run fewer files
at once and use less.

## Size at equal quality

To compare encoders fairly, 8 photos were scaled to about 11 MP and saved as PNG,
which removes the original JPEG compression. This table shows output size
relative to jpegli at the same SSIM, so lower is better.

| Encoder | Encode time | SSIM 0.92 | SSIM 0.94 | SSIM 0.96 | SSIM 0.98 |
|---|---:|---:|---:|---:|---:|
| jpegli | 0.7 s | 1.00 | 1.00 | 1.00 | 1.00 |
| WebP (vpx) | 1.1 s | 1.29 | 1.31 | 1.03 | 0.94 |
| AVIF speed 10 | 6.1 s | 0.70 | 0.70 | 0.75 | 0.79 |

## Decisions

- **Squeeze levels map to per-codec settings.** The same number means different
  things: quality 75 scores SSIM 0.92 on jpegli and 0.97 on AVIF. Levels are
  anchored on jpegli, with WebP and AVIF set to match its SSIM.
- **The JPEG scale was set by eye, not by SSIM.** Camera photos score much lower
  than the small Kodak test images at the same setting, partly because removing
  sensor noise counts as damage. 1:1 crops at q76 looked almost identical to the
  original despite scoring 0.92, so chasing a higher score would have cost twice
  the bytes for nothing visible.
- **Photos default to JPEG, not WebP.** jpegli beat WebP at most quality levels on
  real photos, and JPEG works everywhere.
- **AVIF is opt-in, at speed 10.** Speed 10 keeps most of the savings at a quarter
  of the time of speed 8.
- **Use the wazero backend.** `wasm2go` removes cold start, but made the benchmark
  binary 45 MB against 16.5 MB (both unstripped). Cold start on wazero is 130 ms
  for jpegli and 1 s for AVIF, only on first use.
- **Palette PNG skips Floyd-Steinberg dithering.** On the test images it gave
  larger files and lower SSIM.
- **Size estimates use an 8×8 grid of 96 px tiles.** Across the 22 photos the mean
  error is 5%, and the worst is 16%. Coarser grids miss the subject of a portrait:
  a 4×4 grid was 46% off, because it mostly sampled the backdrop.

## Traps

- **Option structs turn defaults off.** Passing `jpegli.EncodingOptions` zeroes
  `OptimizeCoding` and `AdaptiveQuantization`, so set both explicitly. Subsampling
  needs setting too.
- **`vpx` `Method: 0` means fastest, not default.** Set it explicitly. `Threads: 0`
  means all cores, which multiplies with our workers.
- **Keep straight alpha.** Converting to premultiplied `image.RGBA` added rounding
  noise on soft edges and made a test logo 4x larger (27K to 107K).
- **AVIF memory is invisible to Go.** It lives in WASM memory outside the Go heap,
  so `GOMEMLIMIT` can't limit it, and it stays allocated until a collection runs
  finalizers. Without an explicit `runtime.GC()` after each encode, a batch peaked
  at 26.8 GB on a 27 GB machine, and an unbudgeted test harness was killed by the
  OS. The engine budgets 300 bytes per pixel for AVIF and collects after each file.
- **Lossless PNG re-encoding can grow files.** Neither `pngn` nor `image/png`
  matches oxipng-class tools. Real PNG savings come from palettes or from WebP.
- **Content decides the best format.** With 256 or fewer colors, a palette is
  lossless. Lossless WebP beat lossy WebP on a screenshot test (10K against 199K),
  while lossy WebP beat lossless on a photo (45K against 394K).
- **`jpegn` decodes truncated baseline JPEGs without an error.** squish checks for
  the end of image marker and warns, so a broken file can't look like a success.
- **ICC support is uneven.** `pngn` reads and writes profiles. JPEG and WebP
  profiles are handled in `internal/codec/metadata.go`, because the encoders have
  no API for them. `avif` has none, so squish warns instead.

## Limits of this data

- SSIM here is luma only, and it is not a visual verdict. Eye checks covered 1:1
  crops of two photos, so this is a sensible default rather than proof.
- The camera set leans towards professional photos. Phone photos may differ.
- The screenshot and logo tests used a few real graphics plus generated images.
  Wikimedia rate limits blocked a larger set of real screenshots.
- ICC handling is tested with synthetic profiles, not real wide-gamut photos.
