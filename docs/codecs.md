# Codec choices

Why squish uses the libraries it does. Numbers come from a throwaway benchmark
run in November 2026 on a 16-thread Linux machine (WSL2, Go 1.26.5).

## Stack

| Job | Library | Why |
|---|---|---|
| Decode JPEG | `gen2brain/jpegn` | About 2x faster than `image/jpeg`, applies EXIF rotation |
| Encode JPEG | `gen2brain/jpegli` | Smallest JPEG output at high quality |
| PNG | `gen2brain/pngn` | Keeps ICC and other metadata, which `image/png` drops |
| Palette reduction | `ericpauley/go-quantize` | Median cut, used without dithering |
| WebP | `gen2brain/vpx/webp` | About 4x faster than the WASM encoder, within a few percent on size |
| AVIF | `gen2brain/avif` | Smallest output, but slow and memory hungry, so opt-in |

All of it builds with `CGO_ENABLED=0 -tags nodynamic`.

## Size at equal quality

Output size relative to `image/jpeg` at the same SSIM, on the 8 lossless Kodak
images. Lower is better. Median encode time is per 0.4 MP image.

| Encoder | Encode time | SSIM 0.93 | SSIM 0.95 | SSIM 0.97 |
|---|---|---|---|---|
| `image/jpeg` | 10 ms | 1.00 | 1.00 | 1.00 |
| jpegn | 6 ms | 0.98 | 1.00 | 1.00 |
| jpegli | 28 ms | 0.95 | 0.92 | 0.88 |
| WebP (vpx) | 46 ms | 0.88 | 0.93 | 0.96 |
| WebP (WASM) | 186 ms | 0.86 | 0.92 | 0.94 |
| AVIF speed 10 | 198 ms | 0.66 | 0.67 | 0.69 |
| AVIF speed 8 | 740 ms | 0.62 | 0.63 | 0.65 |
| AVIF speed 6 | 2138 ms | 0.56 | 0.58 | 0.61 |

## Batch speed and memory

| Encoder | Workers | Photos (12 MP) | Time | Peak memory |
|---|---|---|---|---|
| jpegn | 8 | 20 | 1.1 s | 2.0 GB |
| jpegli | 8 | 20 | 3.6 s | 4.0 GB |
| WebP (vpx) | 8 | 20 | 3.8 s | 3.2 GB |
| AVIF speed 8 | 4 | 8 | 39 s | 9.8 GB |
| AVIF speed 8 | 8 | 8 | 16 s | 11.8 GB |

A single 12 MP AVIF encode took 6 s at speed 10, 12 s at speed 8 and 39 s at
speed 6, peaking around 1.4 GB each time.

## Decisions

- **The quality setting must be mapped per codec.** The same number means
  different things: q75 gives SSIM 0.94 on jpegli and 0.98 on AVIF. Starting
  points from the Kodak set:

  | Preset | Target SSIM | jpegli | WebP | AVIF |
  |---|---|---|---|---|
  | Small | 0.93 | 63 | 49 | 49 |
  | Balanced | 0.95 | 76 | 72 | 57 |
  | High | 0.97 | 87 | 85 | 69 |

- **Photos default to JPEG, not WebP.** jpegli matched or beat WebP at the
  higher quality targets, and it stays a universally readable format.
- **AVIF is opt-in, at speed 10, with few workers.** Speed 10 gives most of the
  savings at a quarter of the time of speed 8. Worker count must be limited by
  available memory, not only by CPU count.
- **Use the wazero backend.** `wasm2go` removes cold start but made the benchmark
  binary 45 MB against 16.5 MB (both unstripped). Cold start on wazero is 130 ms
  for jpegli and 1 s for AVIF, only on first use.
- **Palette PNG skips Floyd-Steinberg dithering.** On the test images it gave
  larger files and lower SSIM. Check for visible banding by eye before release.

## Traps

- **Option structs turn defaults off.** Passing `jpegli.EncodingOptions` zeroes
  `OptimizeCoding` and `AdaptiveQuantization`, so set both explicitly. The same
  applies to `avif.Options` subsampling, which needs setting to 4:2:0.
- **`vpx` `Method: 0` means fastest, not default.** Use a negative value or set
  it explicitly. `Threads: 0` means all cores, which multiplies with our workers.
- **Keep straight alpha.** Converting to premultiplied `image.RGBA` added
  rounding noise on soft edges and made a test logo 4x larger (27K to 107K). Use
  `image.NRGBA` for anything with transparency.
- **Lossless PNG re-encoding can grow files.** Neither `pngn` nor `image/png`
  matches oxipng-class tools: an optimized Kodak PNG went from 491K to 527K. Real
  PNG savings come from palette reduction, or from converting to WebP.
- **Content decides the best format.** With 256 or fewer colors, a palette is
  lossless. Lossless WebP beat lossy WebP on the screenshot test (10K against
  199K), while lossy WebP beat lossless on a photo (45K against 394K).
- **ICC support is uneven.** `pngn` reads and writes profiles. `jpegn` writes them
  through `Segments`, but reading means parsing APP2 ourselves. `vpx` and `avif`
  have no public API for writing them.

## Limits of this data

- SSIM here is luma only and is not a visual verdict. Nothing has been compared
  by eye yet, and encoders that use perceptual tricks can score lower than they
  look.
- The Kodak set is 8 small images. Treat the ratios as direction, not as promises.
- The 12 MP photos were recompressed JPEGs, which likely favors JPEG encoders, so
  they are excluded from the size table and are not valid "percent saved" claims.
- The screenshot test image was generated and has only 7 colors. Real screenshots
  have anti-aliased text.
- All five release targets cross-compile at 8 to 10 MB, with fresh builds of about
  10 s, but only linux/amd64 was run. arm64 uses a different jpegli backend, so it
  needs real testing in CI.
