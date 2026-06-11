#!/usr/bin/env python3
"""
Migration script: copy and embed data into Chroma Cloud collections.

Usage:
  python scripts/migrate-to-chroma.py --dir projects/ --collection ai-workstation-knowledge
  python scripts/migrate-to-chroma.py --dir docs/ --collection ai-workstation-docs
  python scripts/migrate-to-chroma.py --dir agents/ --collection ai-workstation-agents

Shards data by top-level directory into separate collections.
Each collection gets a hybrid dense+sparse schema.
Files > 16 KiB are chunked with line-based overlap.
"""

import argparse
import glob
import os
import sys
import time

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from projects.chroma.chunking import chunk_document
from projects.chroma.schema import get_or_create_hybrid_collection


def migrate_directory(
    dir_path: str,
    collection_name: str,
    chunk_size: int = 1000,
    overlap: int = 100,
    dry_run: bool = False,
) -> int:
    collection = get_or_create_hybrid_collection(name=collection_name)

    extensions = (".py", ".go", ".js", ".ts", ".md", ".txt", ".json", ".yaml", ".yml")
    files = []
    for ext in extensions:
        files.extend(glob.glob(os.path.join(dir_path, "**", f"*{ext}"), recursive=True))

    files.sort()
    total = 0
    skipped = 0

    print(f"Found {len(files)} files in '{dir_path}'")

    for file_path in files:
        rel_path = os.path.relpath(file_path)
        file_size = os.path.getsize(file_path)

        if file_size == 0:
            skipped += 1
            continue

        rel_source = rel_path.replace("/", ":").replace("\\", ":")
        chunks = chunk_document(
            file_path,
            source_id=rel_source,
            chunk_size=chunk_size,
            overlap=overlap,
        )

        if dry_run:
            print(f"  [DRY RUN] Would add {len(chunks)} chunk(s) from {rel_path}")
            total += len(chunks)
            continue

        for chunk in chunks:
            try:
                collection.add(
                    ids=[chunk["id"]],
                    documents=[chunk["document"]],
                    metadatas=[chunk["metadata"]],
                )
                total += 1
            except Exception as e:
                print(f"  ERROR adding {chunk['id']}: {e}")
                continue

        if len(chunks) > 1:
            print(f"  Added {len(chunks)} chunks from {rel_path} ({file_size} bytes)")

        if total % 50 == 0 and total > 0:
            print(f"  Progress: {total} chunks added...")

    action = "Would add" if dry_run else "Added"
    print(f"\n{action} {total} chunks to '{collection_name}'")
    if skipped:
        print(f"  Skipped {skipped} empty files")
    return total


def main():
    parser = argparse.ArgumentParser(description="Migrate data to Chroma Cloud")
    parser.add_argument(
        "--dir",
        required=True,
        help="Directory to scan for files",
    )
    parser.add_argument(
        "--collection",
        required=True,
        help="Target Chroma collection name",
    )
    parser.add_argument(
        "--chunk-size",
        type=int,
        default=1000,
        help="Target chunk size in bytes (default: 1000)",
    )
    parser.add_argument(
        "--overlap",
        type=int,
        default=100,
        help="Overlap between chunks in bytes (default: 100)",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Scan and report without adding data",
    )
    parser.add_argument(
        "--all-shards",
        action="store_true",
        help="Migrate all top-level directories as separate shards",
    )

    args = parser.parse_args()

    if args.all_shards:
        base_dir = args.dir
        shards = {}
        for entry in os.listdir(base_dir):
            full = os.path.join(base_dir, entry)
            if os.path.isdir(full) and not entry.startswith("."):
                shards[entry] = full

        print(f"Migrating {len(shards)} shards from '{base_dir}':")
        total = 0
        for name, path in shards.items():
            collection_name = f"{args.collection}-{name}"
            c = migrate_directory(path, collection_name, args.chunk_size, args.overlap, args.dry_run)
            total += c
        print(f"\nTotal: {total} chunks across {len(shards)} collections")
    else:
        migrate_directory(args.dir, args.collection, args.chunk_size, args.overlap, args.dry_run)


if __name__ == "__main__":
    start = time.time()
    main()
    elapsed = time.time() - start
    print(f"Done in {elapsed:.1f}s")
