#!/usr/bin/env python3
"""Preserve a Labcorp report and import readings through existing Bean commands."""
import argparse
import json
import os
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
from source_ingest import IngestError, ingest_source


def ingest(source, log, timezone=''):
    result = ingest_source(source, log, importer=os.environ.get('LABCORP_BIN', 'bean-import-labcorp'),
                           args=['-tz', timezone] if timezone else [], limit=32 << 20,
                           archive_type='lab.report', assurance='unsigned-report')
    result['reading_policy'] = 'existing specimen/test identities dedupe; amendments require explicit correction'
    return result


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source')
    parser.add_argument('log')
    parser.add_argument('--timezone', default='')
    args = parser.parse_args()
    try:
        print(json.dumps(ingest(args.source, args.log, args.timezone), sort_keys=True))
    except IngestError as error:
        parser.exit(1, 'ingestion failed: ' + str(error) + '\n')
