#!/usr/bin/env bash

set -euo pipefail

profile=${1:-coverage.out}

if [[ ! -f "$profile" ]]; then
	echo "ERROR: coverage profile not found: $profile" >&2
	exit 1
fi

awk '
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
		failed = 0
		totalStatements = 0
		totalCovered = 0
		for (file in statements) {
			total = statements[file]
			coveredStatements = covered[file]
			totalStatements += total
			totalCovered += coveredStatements
			if (coveredStatements < total) {
				printf "ERROR: package %s coverage is %d/%d statements (%.1f%%; required 100%%)\n", file, coveredStatements, total, coveredStatements * 100 / total
				failed = 1
			}
		}
		if (totalStatements == 0) {
			print "ERROR: coverage profile contains no statements"
			exit 1
		}
		printf "Total coverage: %d/%d statements (%.1f%%)\n", totalCovered, totalStatements, totalCovered * 100 / totalStatements
		if (totalCovered < totalStatements) {
			failed = 1
		}
		if (failed) {
			print "ERROR: coverage must be 100% for every package and overall"
			exit 1
		}
		print "Coverage gate passed: 100% for every package and overall"
	}
' "$profile"
