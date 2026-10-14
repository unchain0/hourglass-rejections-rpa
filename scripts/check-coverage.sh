#!/usr/bin/env bash

set -euo pipefail

profile=${1:-coverage.out}
threshold=${2:-70}

if [[ ! -f "$profile" ]]; then
	echo "ERROR: coverage profile not found: $profile" >&2
	exit 1
fi

awk -v threshold="$threshold" '
	$1 == "mode:" { next }
	{
		file = $1
		sub(/:[0-9]+\.[0-9]+,[0-9]+\.[0-9]+$/, "", file)
		sub(/\/[^/]+$/, "", file)
		statements[file] += $2
		if ($3 > 0) {
			covered[file] += $2
		}
	}
	END {
		for (f in statements) {
			totalStatements += statements[f]
			totalCovered += covered[f] + 0
		}
		if (totalStatements == 0) {
			print "ERROR: coverage profile contains no statements"
			exit 1
		}
		printf "Total coverage: %d/%d statements (%.1f%%)\n", totalCovered, totalStatements, totalCovered * 100 / totalStatements
		if (totalCovered * 100 / totalStatements < threshold) {
			printf "ERROR: total coverage %.1f%% is below threshold %d%%\n", totalCovered * 100 / totalStatements, threshold
			exit 1
		}
		printf "Coverage gate passed: %.1f%% >= %d%%\n", totalCovered * 100 / totalStatements, threshold
	}
' "$profile"
