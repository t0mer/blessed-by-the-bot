from pydantic import BaseModel
from typing import Optional, List

class Bless(BaseModel):
    BlessId: Optional[int]
    GenderId: int
    LanguageId: int
    Bless: str
