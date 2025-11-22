from llama_index.embeddings.ollama import OllamaEmbedding

embed_model = OllamaEmbedding(
    model_name="nomic-embed-text",
    base_url="http://localhost:11434",
)

embedding = embed_model.get_query_embedding("こんにちは")
print(len(embedding))