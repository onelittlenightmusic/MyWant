# Codebase RAG System

A local SQLite-based Retrieval Augmented Generation (RAG) system for semantic search and code discovery across the MyWant repository.

## Features

- ✅ **Local Storage**: SQLite database - no external services needed
- ✅ **Semantic Search**: Optional AI embeddings for intelligent code search
- ✅ **Hybrid Indexing**: Both file-level and function-level code entities
- ✅ **Architecture Overview**: Understand codebase structure at a glance
- ✅ **Multi-filter Search**: Search by entity type (functions, structs, files, etc.)
- ✅ **Fast Queries**: Optimized SQLite indexing and search

## Quick Start

### 1. Setup

```bash
# Create a virtual environment (optional but recommended)
cd /Users/hiroyukiosaki/work/MyWant
python3 -m venv venv
source venv/bin/activate

# Install dependencies
pip install -r tools/requirements-rag.txt
```

### 2. Index the Codebase

```bash
# Index all Go files in the repository
python3 tools/codebase_rag.py index
```

This will:
- Parse all `.go` files in the repository
- Extract functions, types, structs, and interfaces
- Create semantic embeddings (if sentence-transformers is installed)
- Build SQLite database at `codebase_rag.db`

Output example:
```
🔍 MyWant Codebase RAG System
============================================================
📂 Scanning /Users/hiroyukiosaki/work/MyWant...
📁 Found 45 Go files

📝 Indexing 250+ code entities...
  Progress: 0/250
  Progress: 50/250
  ...
✅ Indexed 256 entities successfully

📐 Architecture Overview:
  Total Entities: 256
  By Type: {'file': 45, 'function': 180, 'struct': 31}

  Top Files:
    - engine/core/chain_builder.go: 25 entities
    - engine/types/travel_types.go: 18 entities
    ...
```

### 3. Search the Codebase

#### Interactive Search

```bash
python3 tools/codebase_rag.py
```

Then try queries like:

```
🔎 Search: Want execution
# Returns functions and files related to Want execution

🔎 Search: func:GetOutputChannel
# Search for specific function

🔎 Search: struct:ChainBuilder
# Search for specific struct

🔎 Search: file:travel
# Search for files matching a pattern

🔎 Search: arch
# Show architecture overview

🔎 Search: help
# Show search options
```

#### Programmatic Search

```python
from codebase_rag import CodebaseRAG

rag = CodebaseRAG("codebase_rag.db")

# Search all entities
results = rag.search("Want execution", limit=10)

# Search specific types
functions = rag.search("GetOutputChannel", entity_types=['function'], limit=5)
structs = rag.search("ChainBuilder", entity_types=['struct'], limit=5)

# Get architecture overview
overview = rag.get_architecture_overview()
print(f"Total entities: {overview['total_entities']}")
print(f"By type: {overview['by_type']}")
print(f"Top files: {overview['top_files']}")

rag.close()
```

## Database Schema

### entities table
```sql
CREATE TABLE entities (
    entity_id TEXT PRIMARY KEY,      -- Unique identifier
    entity_type TEXT,                -- 'file', 'function', 'struct', 'interface', 'method'
    name TEXT,                       -- Entity name
    file_path TEXT,                  -- Relative path to file
    line_number INTEGER,             -- Line number in file
    content TEXT,                    -- Code snippet
    summary TEXT,                    -- Description/comments
    signature TEXT,                  -- Function/type signature
    parent_entity TEXT,              -- Parent entity ID (for nested items)
    embedding BLOB                   -- Vector embedding (binary)
);
```

### search_index table
```sql
CREATE TABLE search_index (
    entity_id TEXT PRIMARY KEY,
    keywords TEXT                    -- Searchable keywords
);
```

## Search Examples

### Use Case 1: Understanding Want Lifecycle

```
🔎 Search: want execution lifecycle
Results:
1. Exec (function)
   📁 engine/core/chain.go:123
   📝 Main execution method for want processing
   ⚙️  func (w *Want) Exec() bool { ...

2. BeginProgressCycle (function)
   📁 engine/core/chain.go:89
   📝 Start batching state changes for execution
   ...
```

### Use Case 2: Finding Channel Communication Patterns

```
🔎 Search: func:GetInputChannel
Results:
1. GetInputChannel (function)
   📁 engine/core/chain_helpers.go:16
   📝 Get input channel by index, returns (channel, connectionAvailable)
   ⚙️  func (n *Want) GetInputChannel(index int) (chain.Chan, bool) {

2. GetFirstInputChannel (function)
   📁 engine/core/chain_helpers.go:48
   📝 Get first input channel from paths
   ...
```

### Use Case 3: Exploring Architecture

```
🔎 Search: arch

📐 Architecture Overview:
  Total Entities: 256

  Entity Types:
    - file: 45
    - function: 180
    - struct: 31
    - interface: 5

  Packages/Directories:
    - engine/src: 95 entities
    - engine/cmd/types: 85 entities
    - recipes: 12 entities
    - capabilities: 8 entities
```

## Performance Notes

- **Indexing**: ~1-2 seconds for full codebase (256+ entities)
- **Search**: < 100ms for typical queries
- **Memory**: ~5MB database size, minimal RAM usage
- **Storage**: SQLite database at `codebase_rag.db` (~5-10MB)

## Installing Optional Dependencies

For enhanced semantic search with AI embeddings:

```bash
pip install sentence-transformers
```

This enables:
- Similarity-based ranking of results
- Better understanding of code semantics
- More relevant search results

Without it, the system still works with keyword-based search.

## Updating the Index

When the codebase changes:

```bash
# Simply re-run indexing
python3 tools/codebase_rag.py index

# The new index will replace the old one
```

## Troubleshooting

### Issue: "sentence-transformers not installed"

The system will still work with keyword-based search. Install it for better results:
```bash
pip install sentence-transformers
```

### Issue: Database locked error

Close any other processes using the database and try again.

### Issue: Out of memory during indexing

Large embeddings can use RAM. You can:
1. Reduce the number of files indexed (edit the parser)
2. Skip function/method extraction in the parser
3. Increase system memory

## Advanced Usage

### Custom Entity Filtering

Modify `GoCodeParser._parse_file()` to skip certain files:

```python
# Skip test files
if "_test.go" in str(go_file):
    continue

# Skip specific directories
if "vendor" in str(go_file):
    continue
```

### Custom Embedding Model

Change the embedding model in `EmbeddingGenerator`:

```python
# Use larger, more accurate model (slower)
self.model = SentenceTransformer('all-mpnet-base-v2')

# Or use faster model
self.model = SentenceTransformer('all-MiniLM-L6-v2')
```

### Export Search Results

```python
import json

rag = CodebaseRAG("codebase_rag.db")
results = rag.search("your query")

# Export to JSON
with open("search_results.json", "w") as f:
    json.dump(results, f, indent=2)
```

## Integration with Claude Code

This RAG system can be used with Claude Code for:

1. **Code Understanding**: Get instant answers about function purposes
2. **Architecture Analysis**: Understand code structure and relationships
3. **Search Assistance**: Find relevant code snippets quickly
4. **Documentation**: Generate documentation from indexed code

Example workflow:
```bash
# Build index
python3 tools/codebase_rag.py index

# Use Claude Code for analysis with search support
# Claude can query the database for context

# Or export results for Claude to analyze
python3 tools/codebase_rag.py --export results.json
```

## Contributing

To extend the RAG system:

1. Add new entity types in `GoCodeParser`
2. Enhance `_create_keywords()` for better search
3. Add specialized search methods to `CodebaseRAG`
4. Contribute back via pull requests

## License

Same as MyWant project
