from pydantic import BaseModel

class Configuration(BaseModel):
    WhatsappApiUrl: str
    WhatsappApiToken: str
    WhatsappApiSessionName: str
