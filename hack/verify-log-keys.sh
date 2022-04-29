#!/bin/bash

invalidLines=0

for f in $(find . -name "*_test.go"); do
  # Find all lines that have a KeysAndValues (ie any test case inspecting set log keys).
  # We have to use pcregrep here so that it captures multi line regexes.
  allKeys=$(pcregrep --line-offsets -M 'KeysAndValues: \[\]interface{}{[^}]*},' ${f})

  # Find all lines that have a compliant set of keys (ie lowerCamelCase).
  # We have to use pcregrep here so that it captures multi line regexes.
  compliantKeys=$(pcregrep --line-offsets -M 'KeysAndValues: \[\]interface{}{(\n)?(\s*\"[a-z][A-Za-z]*\",[^}]*,?\n?)*\n?\s*},' ${f})

  # Find the lines which are only present in the first of these
  nonCompliantLines=$(echo $allKeys $compliantKeys | xargs -n1 | sort | uniq -u)

  for l in $nonCompliantLines; do
    ((invalidLines+=1))
    echo "Found non-compliant log key around ${f} line ${l}. All log keys should be lowerCamelCase."

    # We keep the beginning of the line so sum the start and end of the offset to work out how many characters to print.
    chars=$(echo $l | cut -d: -f2 | awk 'BEGIN {RS="," ; sum = 0 }{sum += $1 }END { print sum }')
    # Print starting at the matched line number, the number of chars we calculated
    tail -n +$(echo $l | cut -d: -f1) ${f} | head -c $chars
    # Ensure a new line at the end
    echo
  done
done

exit $invalidLines
