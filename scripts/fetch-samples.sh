#!/bin/sh
# Downloads public domain camera originals from NASA into samples/. They are
# used for the demo recording and for testing on real photos.
set -eu

dir="${1:-samples}"
mkdir -p "$dir"

base="https://images-assets.nasa.gov/image"
for id in \
  KSC-20170104-PH_GEB01_0010:JPG \
  KSC-20170104-PH_GEB01_0015:JPG \
  KSC-20170504-PH_SWW01_0065:JPG \
  KSC-20170508-PH_SWW01_0025:JPG \
  KSC-20260508-PH-AJN01_0080:JPG \
  iss063e080437:jpg \
  iss069e003275:jpg \
  iss069e025220:jpg; do
  name="${id%:*}"
  ext="${id#*:}"
  out="$dir/$name.jpg"
  [ -f "$out" ] || curl -fsSL -o "$out" "$base/$name/$name~orig.$ext"
done
echo "Samples are in $dir/"
