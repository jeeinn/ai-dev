#!/bin/bash
# Fix files starting with _ for go:embed compatibility
# go:embed ignores files starting with _ or .

cd "$(dirname "$0")/dist/assets"

for f in _*; do
    [ -f "$f" ] || continue
    newname="${f#_}"
    echo "Renaming: $f -> $newname"
    mv "$f" "$newname"
done

# Update references in JS files
cd ..
grep -rl "_plugin-vue" assets/ 2>/dev/null | while read f; do
    sed -i.bak 's/_plugin-vue/plugin-vue/g' "$f"
    rm -f "${f}.bak"
done

echo "Done fixing files for go:embed"
