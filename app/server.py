import time
import uvicorn
import schedule
import threading
from pydantic import BaseModel
from typing import Optional, List
from sqliteconnector import SqliteConnector
from models.bless import Bless
from models.gender import Gender
from models.person import Person
from models.language import Language


from fastapi import FastAPI, Request, File, Form, UploadFile
from fastapi.responses import UJSONResponse
from fastapi.templating import Jinja2Templates
from fastapi.responses import HTMLResponse
from fastapi.staticfiles import StaticFiles
from fastapi.responses import JSONResponse
from starlette.responses import FileResponse
from starlette.responses import StreamingResponse
from fastapi.middleware.cors import CORSMiddleware
from fastapi.encoders import jsonable_encoder
from starlette_exporter import PrometheusMiddleware, handle_metrics



class Server:
    def __init__(self):
        self.app = FastAPI()
        self.db = SqliteConnector()
    
    
    
        @self.app.get("/languages/")
        def get_languages():
            languages = self.db.select_all_languages(True)
            return JSONResponse(languages)


        @self.app.get("/genders/")
        def get_genders():
            genders = self.db.select_all_genders()
            return JSONResponse(genders)
        
        @self.app.get("/blesses/")
        def get_blesses():
            blesses = self.db.select_all_blesses()
            return JSONResponse(blesses)
        
        @self.app.get("/persons/")
        def get_persons():
            persons = self.db.select_all_persons()
            return JSONResponse(persons)
        

        @self.app.delete("/languages/{language_id}")
        def delete_language(language_id: int):
            self.db.delete_language(language_id=language_id)
            return {"message": "Language deleted successfully"}

        @self.app.delete("/genders/{gender_id}")
        def delete_gender(gender_id: int):
            self.db.delete_gender(gender_id=gender_id)
            return {"message": "Gender deleted successfully"}


        @self.app.delete("/blesses/{bless_id}")
        def delete_bless(bless_id: int):
            self.db.delete_bless(bless_id=bless_id)
            return {"message": "Bless deleted successfully"}

        @self.app.delete("/persons/{person_id}")
        def delete_person(person_id: int):
            self.db.delete_person(person_id=person_id)
            return {"message": "Person deleted successfully"}


        @self.app.put("/languages/{language_id}", response_model=Language)
        def update_language(language_id: int, language: Language):
            self.db.update_language(language_id=language_id,new_language=language)
            return {**language.dict(), "LanguageId": language_id}

        @self.app.put("/genders/{gender_id}", response_model=Gender)
        def update_gender(gender_id: int, gender: Gender):
            self.db.update_gender(gender_id=gender_id,gender=gender.Gender)
            return {**gender.dict(), "GenderId": gender_id}

        @self.app.put("/blesses/{bless_id}", response_model=Bless)
        def update_bless(bless_id: int, bless: Bless):
            self.db.update_bless(bless_id=bless_id,gender_id=bless.GenderId,language_id=bless.LanguageId,bless=bless.Bless)
            return {**bless.dict(), "BlessId": bless_id}

        @self.app.put("/persons/{person_id}", response_model=Person)
        def update_person(person_id: int, person: Person):
            self.db.update_person(person_id=person_id,first_name=person.FirstName, last_name=person.LastName,
                                  birth_date=person.BirthDate,gender_id=person.GenderId,language_id=person.LanguageId,
                                  phone_number=person.PhoneNumber,preferred_hour=person.PreferredHour)
            return {**person.dict(), "PersonId": person_id}


        @self.app.post("/languages/", response_model=Language)
        def create_language(language: Language):
            language_id = self.db.insert_language(language=language.Language)
            return {**language.dict(), "LanguageId": language_id}



        @self.app.post("/genders/", response_model=Gender)
        def create_gender(gender: Gender):
            gender_id = self.db.insert_gender(gender=gender_id)
            return {**gender.dict(), "GenderId": gender_id}


        @self.app.post("/blesses/", response_model=Bless)
        def create_bless(bless: Bless):
            bless_id = self.db.insert_bless(gender_id=bless.GenderId,language_id=bless.LanguageId, bless=bless.Bless)
            return {**bless.dict(), "BlessId": bless_id}

        @self.app.post("/persons/", response_model=Person)
        def create_person(person: Person):
            person_id = self.db.insert_person(person.FirstName, person.LastName, person.BirthDate, person.GenderId, person.LanguageId, person.PhoneNumber, person.PreferredHour)
            return {**person.dict(), "PersonId": person_id}
        
    def start(self):
        uvicorn.run(self.app, host="0.0.0.0", port=8082)