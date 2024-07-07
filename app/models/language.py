from pydantic import BaseModel
from typing import Optional, List

class Language(BaseModel):
    LanguageId: Optional[int]
    Language: str