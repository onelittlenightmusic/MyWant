#!/usr/bin/env python3
"""
Codebase RAG System - Index and search Go code semantically
Supports both file-level and function-level indexing for hybrid search
"""

import os
import sqlite3
import json
import hashlib
import re
from pathlib import Path
from typing import List, Dict, Optional, Tuple
from dataclasses import dataclass, asdict
from collections import defaultdict

try:
    from sentence_transformers import SentenceTransformer
    HAS_EMBEDDINGS = True
except ImportError:
    HAS_EMBEDDINGS = False
    print("⚠️  sentence-transformers not installed. Using text-based search only.")
    print("   Install with: pip install sentence-transformers")


@dataclass
class CodeEntity:
    """Represents a code entity (file, function, type, etc)"""
    entity_id: str
    entity_type: str  # 'file', 'function', 'type', 'method', 'struct'
    name: str
    file_path: str
    line_number: int
    content: str  # The code content
    summary: str  # Brief summary
    signature: str  # Function/type signature
    parent_entity: Optional[str] = None  # For nested entities
    embedding: Optional[List[float]] = None


class GoCodeParser:
    """Parse Go code to extract functions, types, and structures"""

    def __init__(self, root_dir: str):
        self.root_dir = Path(root_dir)
        self.entities: List[CodeEntity] = []

    def parse_all(self) -> List[CodeEntity]:
        """Parse all Go files in the repository"""
        go_files = list(self.root_dir.rglob("*.go"))
        print(f"📁 Found {len(go_files)} Go files")

        for go_file in go_files:
            # Skip test files for now (optional)
            if "_test.go" in str(go_file):
                continue
            self._parse_file(go_file)

        return self.entities

    def _parse_file(self, file_path: Path):
        """Parse a single Go file"""
        try:
            content = file_path.read_text(encoding='utf-8', errors='ignore')
            relative_path = file_path.relative_to(self.root_dir)

            # Add file-level entity
            file_summary = self._extract_file_summary(content)
            file_entity = CodeEntity(
                entity_id=f"file:{relative_path}",
                entity_type="file",
                name=file_path.name,
                file_path=str(relative_path),
                line_number=1,
                content=content[:500],  # First 500 chars
                summary=file_summary,
                signature=f"package {self._extract_package(content)}"
            )
            self.entities.append(file_entity)

            # Extract functions and types
            self._extract_functions(content, relative_path, file_entity.entity_id)
            self._extract_types(content, relative_path, file_entity.entity_id)

        except Exception as e:
            print(f"⚠️  Error parsing {file_path}: {e}")

    def _extract_package(self, content: str) -> str:
        """Extract package name from Go file"""
        match = re.search(r'package\s+(\w+)', content)
        return match.group(1) if match else "unknown"

    def _extract_file_summary(self, content: str) -> str:
        """Extract file summary from comments"""
        lines = content.split('\n')
        summary_lines = []

        for line in lines[:20]:  # Check first 20 lines
            if line.strip().startswith("//"):
                summary_lines.append(line.strip("//").strip())
            elif line.strip() and not line.strip().startswith("package"):
                break

        return " ".join(summary_lines)[:200] if summary_lines else "Go package"

    def _extract_functions(self, content: str, file_path: Path, parent_id: str):
        """Extract function definitions from Go code"""
        # Match function signatures
        func_pattern = r'func\s*(?:\([^)]*\)\s*)?(\w+)\s*\([^)]*\)\s*(?:\([^)]*\))?\s*\{'

        for match in re.finditer(func_pattern, content):
            func_name = match.group(1)
            line_number = content[:match.start()].count('\n') + 1

            # Extract function signature and body
            signature, body = self._extract_code_block(content, match.start(), match.end())

            # Skip test functions
            if func_name.startswith("Test"):
                continue

            entity = CodeEntity(
                entity_id=f"func:{file_path}:{func_name}:{line_number}",
                entity_type="function",
                name=func_name,
                file_path=str(file_path),
                line_number=line_number,
                content=body[:300],
                summary=f"Function {func_name}",
                signature=signature,
                parent_entity=parent_id
            )
            self.entities.append(entity)

    def _extract_types(self, content: str, file_path: Path, parent_id: str):
        """Extract type definitions (structs, interfaces) from Go code"""
        # Match struct definitions
        struct_pattern = r'type\s+(\w+)\s+struct\s*\{'
        interface_pattern = r'type\s+(\w+)\s+interface\s*\{'

        for pattern, type_name in [(struct_pattern, "struct"), (interface_pattern, "interface")]:
            for match in re.finditer(pattern, content):
                name = match.group(1)
                line_number = content[:match.start()].count('\n') + 1

                signature, body = self._extract_code_block(content, match.start(), match.end())

                entity = CodeEntity(
                    entity_id=f"{type_name}:{file_path}:{name}:{line_number}",
                    entity_type=type_name,
                    name=name,
                    file_path=str(file_path),
                    line_number=line_number,
                    content=body[:300],
                    summary=f"{type_name.capitalize()} {name}",
                    signature=signature,
                    parent_entity=parent_id
                )
                self.entities.append(entity)

    def _extract_code_block(self, content: str, start: int, end: int) -> Tuple[str, str]:
        """Extract code block from content"""
        # Find the opening brace
        brace_pos = content.find('{', end)
        if brace_pos == -1:
            return content[start:end], ""

        # Extract signature
        signature = content[start:brace_pos].strip()

        # Find matching closing brace
        depth = 1
        pos = brace_pos + 1
        while pos < len(content) and depth > 0:
            if content[pos] == '{':
                depth += 1
            elif content[pos] == '}':
                depth -= 1
            pos += 1

        body = content[brace_pos:min(pos, brace_pos + 500)]  # Limit body to 500 chars
        return signature, body


class EmbeddingGenerator:
    """Generate embeddings for code entities"""

    def __init__(self):
        self.model = None
        if HAS_EMBEDDINGS:
            try:
                print("🔄 Loading embedding model...")
                self.model = SentenceTransformer('all-MiniLM-L6-v2')  # Fast, lightweight
                print("✅ Embedding model loaded")
            except Exception as e:
                print(f"⚠️  Could not load embedding model: {e}")

    def generate(self, entity: CodeEntity) -> Optional[List[float]]:
        """Generate embedding for a code entity"""
        if not self.model:
            return None

        try:
            # Use entity summary and signature for embedding
            text = f"{entity.name} {entity.entity_type} {entity.summary} {entity.signature}"
            embedding = self.model.encode(text, convert_to_tensor=False)
            return embedding.tolist()
        except Exception as e:
            print(f"⚠️  Error embedding {entity.name}: {e}")
            return None


class CodebaseRAG:
    """Main RAG system for codebase"""

    def __init__(self, db_path: str = "codebase.db"):
        self.db_path = db_path
        self.conn = None
        self.cursor = None
        self._init_db()

    def _init_db(self):
        """Initialize SQLite database"""
        self.conn = sqlite3.connect(self.db_path)
        self.cursor = self.conn.cursor()

        # Create tables
        self.cursor.execute("""
            CREATE TABLE IF NOT EXISTS entities (
                entity_id TEXT PRIMARY KEY,
                entity_type TEXT,
                name TEXT,
                file_path TEXT,
                line_number INTEGER,
                content TEXT,
                summary TEXT,
                signature TEXT,
                parent_entity TEXT,
                embedding BLOB
            )
        """)

        self.cursor.execute("""
            CREATE TABLE IF NOT EXISTS search_index (
                entity_id TEXT PRIMARY KEY,
                keywords TEXT,
                FOREIGN KEY (entity_id) REFERENCES entities (entity_id)
            )
        """)

        self.conn.commit()

    def index_entities(self, entities: List[CodeEntity]):
        """Index code entities into the database"""
        print(f"\n📝 Indexing {len(entities)} code entities...")

        embedding_gen = EmbeddingGenerator()

        for i, entity in enumerate(entities):
            if i % 50 == 0:
                print(f"  Progress: {i}/{len(entities)}")

            # Generate embedding
            embedding = embedding_gen.generate(entity)

            # Convert embedding to bytes
            embedding_bytes = None
            if embedding:
                import struct
                embedding_bytes = struct.pack(f'{len(embedding)}d', *embedding)

            # Insert entity
            self.cursor.execute("""
                INSERT OR REPLACE INTO entities
                (entity_id, entity_type, name, file_path, line_number, content, summary, signature, parent_entity, embedding)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            """, (
                entity.entity_id,
                entity.entity_type,
                entity.name,
                entity.file_path,
                entity.line_number,
                entity.content,
                entity.summary,
                entity.signature,
                entity.parent_entity,
                embedding_bytes
            ))

            # Create search keywords
            keywords = self._create_keywords(entity)
            self.cursor.execute("""
                INSERT OR REPLACE INTO search_index (entity_id, keywords)
                VALUES (?, ?)
            """, (entity.entity_id, keywords))

        self.conn.commit()
        print(f"✅ Indexed {len(entities)} entities successfully\n")

    def _create_keywords(self, entity: CodeEntity) -> str:
        """Create searchable keywords for an entity"""
        words = []

        # Add name parts
        words.extend(re.findall(r'\b\w+\b', entity.name))

        # Add type
        words.append(entity.entity_type)

        # Add summary words
        words.extend(re.findall(r'\b\w+\b', entity.summary)[:5])

        # Add file path components
        words.extend(entity.file_path.split('/'))

        return " ".join(words).lower()

    def search(self, query: str, limit: int = 10, entity_types: Optional[List[str]] = None) -> List[Dict]:
        """Search for code entities"""
        results = []

        # Text-based search
        sql = "SELECT * FROM entities WHERE 1=1"
        params = []

        # Add entity type filter
        if entity_types:
            placeholders = ",".join(["?" for _ in entity_types])
            sql += f" AND entity_type IN ({placeholders})"
            params.extend(entity_types)

        # Add keyword search
        keywords = query.lower().split()
        search_index = self.cursor.execute("SELECT entity_id FROM search_index").fetchall()

        matching_ids = []
        for (entity_id,) in search_index:
            keywords_str = self.cursor.execute(
                "SELECT keywords FROM search_index WHERE entity_id = ?", (entity_id,)
            ).fetchone()

            if keywords_str:
                keywords_text = keywords_str[0].lower()
                match_count = sum(1 for kw in keywords if kw in keywords_text)
                if match_count > 0:
                    matching_ids.append((entity_id, match_count))

        # Sort by relevance
        matching_ids.sort(key=lambda x: x[1], reverse=True)

        # Fetch full entities
        for entity_id, _ in matching_ids[:limit]:
            entity_data = self.cursor.execute(
                "SELECT * FROM entities WHERE entity_id = ?", (entity_id,)
            ).fetchone()

            if entity_data:
                results.append({
                    'entity_id': entity_data[0],
                    'entity_type': entity_data[1],
                    'name': entity_data[2],
                    'file_path': entity_data[3],
                    'line_number': entity_data[4],
                    'content': entity_data[5],
                    'summary': entity_data[6],
                    'signature': entity_data[7]
                })

        return results

    def get_architecture_overview(self) -> Dict:
        """Get architecture overview of the codebase"""
        stats = {
            'total_entities': 0,
            'by_type': defaultdict(int),
            'by_package': defaultdict(int),
            'top_files': []
        }

        # Get counts
        entities = self.cursor.execute("SELECT entity_type, file_path FROM entities").fetchall()

        for entity_type, file_path in entities:
            stats['total_entities'] += 1
            stats['by_type'][entity_type] += 1

            # Extract package from path
            parts = file_path.split('/')
            package = parts[0] if parts else "root"
            stats['by_package'][package] += 1

        # Get top files by entity count
        file_counts = self.cursor.execute("""
            SELECT file_path, COUNT(*) as count FROM entities
            GROUP BY file_path ORDER BY count DESC LIMIT 10
        """).fetchall()

        stats['top_files'] = [{'file': f, 'count': c} for f, c in file_counts]

        return dict(stats)

    def close(self):
        """Close database connection"""
        if self.conn:
            self.conn.close()


def main():
    """Main function to build and use the RAG system"""
    import sys

    repo_root = "/Users/hiroyukiosaki/work/MyWant"
    db_path = "/Users/hiroyukiosaki/work/MyWant/codebase_rag.db"

    print("🔍 MyWant Codebase RAG System")
    print("=" * 60)

    # Initialize RAG
    rag = CodebaseRAG(db_path)

    # Check if we're indexing or searching
    if len(sys.argv) > 1 and sys.argv[1] == "index":
        # Index the codebase
        print(f"📂 Scanning {repo_root}...")
        parser = GoCodeParser(repo_root)
        entities = parser.parse_all()

        print(f"\n📊 Found {len(entities)} code entities:")
        print("  - Files:", sum(1 for e in entities if e.entity_type == "file"))
        print("  - Functions:", sum(1 for e in entities if e.entity_type == "function"))
        print("  - Types:", sum(1 for e in entities if e.entity_type in ["struct", "interface"]))

        # Index into database
        rag.index_entities(entities)

        # Show overview
        overview = rag.get_architecture_overview()
        print("\n📐 Architecture Overview:")
        print(f"  Total Entities: {overview['total_entities']}")
        print(f"  By Type: {dict(overview['by_type'])}")
        print(f"\n  Top Files:")
        for file_info in overview['top_files'][:5]:
            print(f"    - {file_info['file']}: {file_info['count']} entities")

        print(f"\n✅ Database saved to: {db_path}")

    else:
        # Interactive search mode
        print(f"💾 Using database: {db_path}")
        print("Enter search queries (or 'quit' to exit, 'arch' for architecture, 'help' for options)\n")

        try:
            while True:
                query = input("🔎 Search: ").strip()

                if query.lower() == 'quit':
                    break
                elif query.lower() == 'arch':
                    overview = rag.get_architecture_overview()
                    print("\n📐 Architecture Overview:")
                    print(f"  Total Entities: {overview['total_entities']}")
                    print(f"  Entity Types:")
                    for etype, count in sorted(overview['by_type'].items()):
                        print(f"    - {etype}: {count}")
                    print(f"\n  Packages/Directories:")
                    for pkg, count in sorted(overview['by_package'].items()):
                        print(f"    - {pkg}: {count} entities")
                elif query.lower() == 'help':
                    print("\n📖 Search Options:")
                    print("  - Plain query: search by keywords")
                    print("  - 'func:name': search functions")
                    print("  - 'struct:name': search structs")
                    print("  - 'file:name': search files")
                    print("  - 'arch': show architecture")
                    print("  - 'quit': exit\n")
                elif not query:
                    continue
                else:
                    # Parse search options
                    entity_types = None
                    search_query = query

                    if ':' in query:
                        prefix, search_query = query.split(':', 1)
                        if prefix == 'func':
                            entity_types = ['function']
                        elif prefix == 'struct':
                            entity_types = ['struct']
                        elif prefix == 'file':
                            entity_types = ['file']

                    results = rag.search(search_query, limit=10, entity_types=entity_types)

                    if results:
                        print(f"\n✅ Found {len(results)} results:\n")
                        for i, result in enumerate(results, 1):
                            print(f"{i}. {result['name']} ({result['entity_type']})")
                            print(f"   📁 {result['file_path']}:{result['line_number']}")
                            print(f"   📝 {result['summary']}")
                            if result['signature']:
                                sig = result['signature'][:80]
                                print(f"   ⚙️  {sig}...")
                            print()
                    else:
                        print("❌ No results found.\n")

        except KeyboardInterrupt:
            print("\n👋 Goodbye!")

    rag.close()


if __name__ == "__main__":
    main()
