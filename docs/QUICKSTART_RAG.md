# Codebase RAG Quick Start

Your MyWant repository now has a **Retrieval Augmented Generation (RAG)** database for searching and understanding the codebase!

## 📊 Current Status

✅ **Database Built**: `codebase_rag.db` (632KB)
- **Total Entities**: 760
  - Functions: 409
  - Structs/Types: 198
  - Interfaces: 15
  - Files: 138

## 🚀 Quick Usage

### Method 1: Direct Python Search (Easiest)

```python
from tools.codebase_rag import CodebaseRAG

rag = CodebaseRAG("codebase_rag.db")

# Search for anything
results = rag.search("Want execution", limit=10)

# Search by type
functions = rag.search("GetOutputChannel", entity_types=['function'])
structs = rag.search("ChainBuilder", entity_types=['struct'])

# Get architecture overview
overview = rag.get_architecture_overview()

rag.close()
```

### Method 2: Interactive CLI (Most Fun)

```bash
# Simple search
python3 tools/codebase_rag.py

# Then type queries:
# "Want execution" - search keywords
# "func:GetOutputChannel" - search functions
# "struct:ChainBuilder" - search structs
# "file:travel" - search files
# "arch" - show architecture
```

### Method 3: Using the Wrapper Script

```bash
# Build index
bash tools/rag index

# Search
bash tools/rag search "Want execution"

# Show architecture
bash tools/rag arch

# Interactive mode
bash tools/rag interactive
```

## 📚 Search Examples

### Understanding Channel Communication

```python
results = rag.search("GetInputChannel", entity_types=['function'])
# Returns:
# - GetInputChannel (function) in chain_helpers.go
# - Usage patterns and related functions
```

### Finding Chain Builder Implementation

```python
results = rag.search("ChainBuilder", entity_types=['struct'])
# Returns:
# - ChainBuilder struct definition
# - Related methods and functionality
# - Dependencies and connections
```

### Exploring Recipe System

```python
results = rag.search("recipe", limit=20)
# Returns all recipe-related code:
# - Recipe loader functions
# - Recipe parsing logic
# - Recipe type definitions
```

### Architecture Analysis

```python
overview = rag.get_architecture_overview()

# See which files have the most code
for file_info in overview['top_files'][:10]:
    print(f"{file_info['file']}: {file_info['count']} entities")

# Understand entity distribution
print(overview['by_type'])  # {'function': 409, 'struct': 198, ...}
```

## 🔍 Common Searches

Here are useful searches to understand the codebase:

```python
# Core Want System
rag.search("Exec", entity_types=['function'])  # Want execution
rag.search("BeginExecCycle", entity_types=['function'])  # State management
rag.search("GetState", entity_types=['function'])  # State access

# Channel Communication
rag.search("GetInputChannel", entity_types=['function'])  # Input handling
rag.search("GetOutputChannel", entity_types=['function'])  # Output sending
rag.search("Paths", entity_types=['struct'])  # Path information

# Chain Building
rag.search("ChainBuilder", entity_types=['struct'])  # Main builder
rag.search("AddDynamicNode", entity_types=['function'])  # Dynamic nodes
rag.search("Connect", entity_types=['function'])  # Connection logic

# Recipe System
rag.search("GenericRecipe", entity_types=['struct'])  # Recipe format
rag.search("LoadRecipe", entity_types=['function'])  # Recipe loading
rag.search("RegisterWantType", entity_types=['function'])  # Type registration

# Agent System
rag.search("Agent", entity_types=['struct'])  # Agent implementation
rag.search("Exec", entity_types=['function'])  # Agent execution

# Travel Planning
rag.search("RestaurantWant", entity_types=['struct'])  # Restaurant logic
rag.search("TravelCoordinator", entity_types=['struct'])  # Coordination

# Queue System
rag.search("Numbers", entity_types=['struct'])  # Number generator
rag.search("Queue", entity_types=['struct'])  # Queue processor
rag.search("Sink", entity_types=['struct'])  # Data collector
```

## 📈 Advanced Usage

### Export Results to JSON

```python
import json

results = rag.search("ChainBuilder", limit=20)
with open("search_results.json", "w") as f:
    json.dump(results, f, indent=2)
```

### Find All Functions in a File

```python
results = rag.search("chain_builder.go", entity_types=['function'])
# Returns all functions defined in that file
```

### Compare Entity Types

```python
overview = rag.get_architecture_overview()

# What percentage of code is functions vs types?
total = overview['total_entities']
funcs = overview['by_type'].get('function', 0)
types = overview['by_type'].get('struct', 0)

print(f"Functions: {funcs/total*100:.1f}%")
print(f"Structs: {types/total*100:.1f}%")
```

## 💡 Use Cases

### 1. Learning the Codebase
```python
# Find the main entry points
rag.search("main", entity_types=['function'])

# Understand Want lifecycle
rag.search("Exec", limit=20)  # All Exec implementations
```

### 2. Finding Related Code
```python
# Find all channel-related functions
rag.search("Channel", entity_types=['function'])

# Find all types related to State
rag.search("State", entity_types=['struct'])
```

### 3. Architecture Analysis
```python
# Which files are most complex?
overview = rag.get_architecture_overview()

# See the core modules
for pkg, count in overview['by_package'].items():
    if count > 10:
        print(f"{pkg}: {count} entities (core module)")
```

### 4. Code Review
Before submitting code, search for similar patterns:
```python
# Check if similar function names exist
rag.search("MyNewFunction", limit=5)

# Find similar struct patterns
rag.search("WantType", entity_types=['struct'])
```

## 🔧 Rebuilding the Index

When you add new code to the repository:

```bash
# Option 1: Direct rebuild
python3 tools/codebase_rag.py index

# Option 2: Using wrapper
bash tools/rag reset  # Deletes and rebuilds
bash tools/rag index  # Just rebuilds
```

## 📂 Files Created

- `tools/codebase_rag.py` - Main RAG system (760 lines)
- `tools/README_RAG.md` - Detailed documentation
- `tools/requirements-rag.txt` - Python dependencies
- `tools/rag` - Bash wrapper script
- `codebase_rag.db` - SQLite database (632KB)

## 🎯 Next Steps

1. **Explore the codebase**:
   ```python
   from tools.codebase_rag import CodebaseRAG
   rag = CodebaseRAG("codebase_rag.db")
   results = rag.search("Want", limit=20)
   ```

2. **Enhance with embeddings**:
   ```bash
   pip install sentence-transformers
   python3 tools/codebase_rag.py index
   ```

3. **Integrate with your workflow**:
   - Use in documentation generation
   - Reference during code reviews
   - Search during debugging

## 📞 Help

For detailed documentation, see: `tools/README_RAG.md`

For database schema information, see: `tools/README_RAG.md` (Database Schema section)

## 🚨 Troubleshooting

**Q: "Database locked" error**
- Close any other processes using the database
- Or simply restart your search session

**Q: No results found**
- Try simpler search terms
- Use specific entity types: `entity_types=['function']`
- Check spelling carefully

**Q: Want better search results?**
- Install sentence-transformers for semantic search:
  ```bash
  pip install sentence-transformers
  python3 tools/codebase_rag.py index
  ```

**Q: Database is stale after code changes**
- Rebuild: `python3 tools/codebase_rag.py index`
- Takes ~2-3 seconds for full codebase
