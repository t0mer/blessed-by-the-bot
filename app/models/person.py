from pydantic import BaseModel
from typing import Optional, List

class Person(BaseModel):
    PersonId: Optional[int]
    FirstName: str
    LastName: str
    BirthDate: str
    GenderId: int
    LanguageId: int
    PhoneNumber: str
    PreferredHour: Optional[int]