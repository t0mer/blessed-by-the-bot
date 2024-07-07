from pydantic import BaseModel
from typing import Optional, List

class Gender(BaseModel):
    GenderId: Optional[int]
    Gender: str