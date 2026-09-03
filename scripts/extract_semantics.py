import sqlite3
import sqlite_vec
import os
import json
import re
from pathlib import Path

# Connect to Hermes memory database (adjust path if needed, we'll use a specific db for anigo vectors)
DB_PATH = os.path.expanduser("~/.hermes/anigo_vectors.db")
os.makedirs(os.path.dirname(DB_PATH), exist_ok=True)

db = sqlite3.connect(DB_PATH)
db.enable_load_extension(True)
sqlite_vec.load(db)

# Create tables
db.execute("""
CREATE TABLE IF NOT EXISTS code_nodes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    file_path TEXT,
    node_type TEXT, -- 'interface', 'struct', 'function'
    name TEXT,
    content TEXT,
    signature TEXT
);
""")

db.execute("""
CREATE VIRTUAL TABLE IF NOT EXISTS vec_code_nodes USING vec0(
    id INTEGER PRIMARY KEY,
    embedding float[384] -- Placeholder dimension, we will mock embeddings for now or use a local small model if available
);
""")
db.commit()

# Simple Go parser regexes (very naive for demonstration)
INTERFACE_RE = re.compile(r'type\s+(\w+)\s+interface\s*\{([^}]*)\}', re.MULTILINE)
STRUCT_RE = re.compile(r'type\s+(\w+)\s+struct\s*\{([^}]*)\}', re.MULTILINE)

def extract_from_file(filepath):
    with open(filepath, 'r', encoding='utf-8') as f:
        content = f.read()
    
    nodes = []
    
    for match in INTERFACE_RE.finditer(content):
        nodes.append({
            'file_path': filepath,
            'node_type': 'interface',
            'name': match.group(1),
            'content': match.group(0),
            'signature': f"type {match.group(1)} interface"
        })
        
    for match in STRUCT_RE.finditer(content):
        nodes.append({
            'file_path': filepath,
            'node_type': 'struct',
            'name': match.group(1),
            'content': match.group(0),
            'signature': f"type {match.group(1)} struct"
        })
        
    return nodes

def main():
    target_dir = "/root/x/Ani-Go/internal/core"
    all_nodes = []
    for root, _, files in os.walk(target_dir):
        for f in files:
            if f.endswith('.go'):
                filepath = os.path.join(root, f)
                nodes = extract_from_file(filepath)
                all_nodes.extend(nodes)
                print(f"Extracted {len(nodes)} nodes from {f}")
                
    # Insert into DB
    for node in all_nodes:
        # Check if exists
        cur = db.execute("SELECT id FROM code_nodes WHERE file_path = ? AND name = ?", (node['file_path'], node['name']))
        row = cur.fetchone()
        if row:
            db.execute("UPDATE code_nodes SET content = ?, signature = ? WHERE id = ?", (node['content'], node['signature'], row[0]))
            print(f"Updated {node['name']}")
        else:
            db.execute("INSERT INTO code_nodes (file_path, node_type, name, content, signature) VALUES (?, ?, ?, ?, ?)", 
                      (node['file_path'], node['node_type'], node['name'], node['content'], node['signature']))
            print(f"Inserted {node['name']}")
            
    db.commit()
    print("Done! Note: Embeddings generation is skipped in this stub script, but the structured metadata is stored.")

if __name__ == "__main__":
    main()
