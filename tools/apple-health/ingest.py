#!/usr/bin/env python3
"""Preserve Apple Health exports through existing Bean commands; no identity keys."""
import argparse
import json
import os
from pathlib import Path
import sys
import subprocess  # Shared module also used by fault-injection tests.

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
from source_ingest import IngestError, digest, ingest_source


def ingest(source, log, records=False, types=''):
    args = (['-records'] if records else []) + (['-types', types] if types else [])
    return ingest_source(source, log, importer=os.environ.get('APPLE_HEALTH_BIN', 'bean-import-apple-health'),
                         args=args, limit=256 << 20, archive_type='apple-health.export', assurance='unsigned-export')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source')
    parser.add_argument('log')
    parser.add_argument('--records', action='store_true')
    parser.add_argument('--types', default='')
    args = parser.parse_args()
    try:
        print(json.dumps(ingest(args.source, args.log, args.records, args.types), sort_keys=True))
    except IngestError as error:
        # Do not expose file contents, paths, health values or identity secrets.
        parser.exit(1, 'ingestion failed: ' + str(error) + '\n')
