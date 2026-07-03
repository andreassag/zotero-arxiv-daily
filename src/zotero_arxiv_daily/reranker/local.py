import json
import logging
import os
import warnings

import numpy as np
from loguru import logger

from ..utils import add_embedding_tokens, count_tokens_batch
from .base import BaseReranker, register_reranker


@register_reranker("local")
class LocalReranker(BaseReranker):
    def get_similarity_score(self, s1: list[str], s2: list[str]) -> np.ndarray:
        # Save the input data for debugging/inspection
        self._save_embedding_input_data(s1, s2)

        from sentence_transformers import SentenceTransformer

        if not self.config.executor.debug:
            from huggingface_hub.utils import logging as hf_logging
            from transformers.utils import logging as transformers_logging

            transformers_logging.set_verbosity_error()
            hf_logging.set_verbosity_error()
            logging.getLogger("sentence_transformers").setLevel(logging.ERROR)
            logging.getLogger("sentence_transformers.SentenceTransformer").setLevel(logging.ERROR)
            logging.getLogger("transformers").setLevel(logging.ERROR)
            logging.getLogger("huggingface_hub").setLevel(logging.ERROR)
            logging.getLogger("huggingface_hub.utils._http").setLevel(logging.ERROR)
            warnings.filterwarnings("ignore", category=FutureWarning)

        encoder = SentenceTransformer(self.config.reranker.local.model, trust_remote_code=True)
        if self.config.reranker.local.encode_kwargs:
            encode_kwargs = self.config.reranker.local.encode_kwargs
        else:
            encode_kwargs = {}

        # Count tokens for embedding input using Qwen tokenizer
        tokenizer_type = self.config.reranker.local.get("tokenizer_type", "qwen")
        all_texts = s1 + s2

        # Count tokens for all texts
        token_counts = count_tokens_batch(all_texts, tokenizer_type)
        total_tokens = sum(token_counts)
        add_embedding_tokens(total_tokens)
        logger.debug(f"Local embedding token count: {total_tokens}")

        s1_feature = encoder.encode(s1, **encode_kwargs, show_progress_bar=True)
        s2_feature = encoder.encode(s2, **encode_kwargs, show_progress_bar=True)
        sim = encoder.similarity(s1_feature, s2_feature)
        return sim.numpy()

    def _save_embedding_input_data(self, s1: list[str], s2: list[str]):
        """Save the input data (s1 and s2) to files for debugging and inspection."""
        try:
            # Create outputs directory if it doesn't exist
            os.makedirs("outputs", exist_ok=True)

            # Save s1 (candidate abstracts)
            s1_path = "outputs/embedding_input_s1.json"
            with open(s1_path, "w", encoding="utf-8") as f:
                json.dump(s1, f, ensure_ascii=False, indent=2)
            logger.info(f"Saved s1 data to: {s1_path}")

            # Save s2 (corpus abstracts)
            s2_path = "outputs/embedding_input_s2.json"
            with open(s2_path, "w", encoding="utf-8") as f:
                json.dump(s2, f, ensure_ascii=False, indent=2)
            logger.info(f"Saved s2 data to: {s2_path}")

        except Exception as e:
            logger.error(f"Failed to save embedding input data: {e}")
