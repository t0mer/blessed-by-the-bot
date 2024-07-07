import os
import time
import shutil
import uvicorn
import schedule
import threading
from zipfile import ZipFile
from loguru import logger
from pydantic import BaseModel
from typing import Optional, List
from sqliteconnector import SqliteConnector
from models.bless import Bless
from models.gender import Gender
from models.person import Person
from models.language import Language
from fastapi import FastAPI, Request, File, Form, UploadFile, HTTPException, BackgroundTasks
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
        self.db = SqliteConnector()
        self.tags_metadata = [
            {
                "name": "Blesses",
                "description": "Blesses API endpoints (Add, Update, Delete, Get)",
            },
            {
                "name": "Persons",
                "description": "Persons API endpoints (Add, Update, Delete, Get)",
            },
            {
                "name": "Genders",
                "description": "Genders API endpoints (Add, Update, Delete, Get)",
            },
            {
                "name": "Languages",
                "description": "Languages API endpoints (Add, Update, Delete, Get)",
            },
            {
                "name": "Utils",
                "description": "Utilitis API endpoints (Database backup/restore, Configuration, etc.)",
            },
        
        ]
        self.app = FastAPI(title="Blessed by the bot | Tomer Klein", description="Whatsapp bot for automated blesses and whishes by @Tomer Klein", version='1.0.0', openapi_tags=self.tags_metadata, contact={"name": "Tomer Klein", "email": "tomer.klein@gmail.com", "url": "https://github.com/t0mer/blessed-by-the-bot"})
        self.app.add_route("/metrics", handle_metrics)
        self.origins = ["*"]

        self.app.add_middleware(
            CORSMiddleware,
            allow_origins=self.origins,
            allow_credentials=True,
            allow_methods=["*"],
            allow_headers=["*"],
        )    


   
            
        @self.app.get("/languages/",tags=['Languages'], summary="Get the list of languages")
        def get_languages():
            languages = self.db.select_all_languages(True)
            return JSONResponse(languages)


        @self.app.get("/genders/", tags=['Genders'], summary="Get the list of genders")
        def get_genders():
            genders = self.db.select_all_genders()
            return JSONResponse(genders)
        
        @self.app.get("/blesses/", tags=['Blesses'], summary="Get the list of blesses")
        def get_blesses():
            blesses = self.db.select_all_blesses()
            return JSONResponse(blesses)
        
        @self.app.get("/persons/", tags=['Persons'], summary="Get the list of persons")
        def get_persons():
            persons = self.db.select_all_persons()
            return JSONResponse(persons)
        

        @self.app.delete("/languages/{language_id}",tags=['Languages'], summary="Delete the requested language")
        def delete_language(language_id: int):
            self.db.delete_language(language_id=language_id)
            return {"message": "Language deleted successfully"}

        @self.app.delete("/genders/{gender_id}", tags=['Genders'], summary="Delete the requested gender")
        def delete_gender(gender_id: int):
            self.db.delete_gender(gender_id=gender_id)
            return {"message": "Gender deleted successfully"}


        @self.app.delete("/blesses/{bless_id}", tags=['Blesses'], summary="Delete the requested bless")
        def delete_bless(bless_id: int):
            self.db.delete_bless(bless_id=bless_id)
            return {"message": "Bless deleted successfully"}

        @self.app.delete("/persons/{person_id}", tags=['Persons'], summary="Delete the requested person")
        def delete_person(person_id: int):
            self.db.delete_person(person_id=person_id)
            return {"message": "Person deleted successfully"}


        @self.app.put("/languages/{language_id}", response_model=Language, tags=['Languages'], summary="Update the language name")
        def update_language(language_id: int, language: Language):
            self.db.update_language(language_id=language_id,new_language=language)
            return {**language.dict(), "LanguageId": language_id}

        @self.app.put("/genders/{gender_id}", response_model=Gender, tags=['Genders'], summary="Update the gender name")
        def update_gender(gender_id: int, gender: Gender):
            self.db.update_gender(gender_id=gender_id,gender=gender.Gender)
            return {**gender.dict(), "GenderId": gender_id}

        @self.app.put("/blesses/{bless_id}", response_model=Bless, tags=['Blesses'], summary="Update the bless")
        def update_bless(bless_id: int, bless: Bless):
            self.db.update_bless(bless_id=bless_id,gender_id=bless.GenderId,language_id=bless.LanguageId,bless=bless.Bless)
            return {**bless.dict(), "BlessId": bless_id}

        @self.app.put("/persons/{person_id}", response_model=Person, tags=['Persons'], summary="Update the person")
        def update_person(person_id: int, person: Person):
            self.db.update_person(person_id=person_id,first_name=person.FirstName, last_name=person.LastName,
                                  birth_date=person.BirthDate,gender_id=person.GenderId,language_id=person.LanguageId,
                                  phone_number=person.PhoneNumber,preferred_hour=person.PreferredHour, intro=person.Intro)
            return {**person.dict(), "PersonId": person_id}


        @self.app.post("/languages/", response_model=Language,tags=['Languages'], summary="Add a new language to the languages list")
        def create_language(language: Language):
            language_id = self.db.insert_language(language=language.Language)
            return {**language.dict(), "LanguageId": language_id}



        @self.app.post("/genders/", response_model=Gender, tags=['Genders'], summary="Add a new gender to the genders list")
        def create_gender(gender: Gender):
            gender_id = self.db.insert_gender(gender=gender_id)
            return {**gender.dict(), "GenderId": gender_id}


        @self.app.post("/blesses/", response_model=Bless, tags=['Blesses'], summary="Add a new bless to the blesses list")
        def create_bless(bless: Bless):
            bless_id = self.db.insert_bless(gender_id=bless.GenderId,language_id=bless.LanguageId, bless=bless.Bless)
            return {**bless.dict(), "BlessId": bless_id}

        @self.app.post("/persons/", response_model=Person, tags=['Persons'], summary="Add a new person to the persons list")
        def create_person(person: Person):
            person_id = self.db.insert_person(person.FirstName, person.LastName, person.BirthDate, person.GenderId, person.LanguageId, person.PhoneNumber, person.PreferredHour, person.Intro)
            return {**person.dict(), "PersonId": person_id}



        @self.app.get("/backup", tags=['Utils'], summary="Create a database backup")
        async def create_backup(background_tasks: BackgroundTasks):
            zip_file = "backup.zip"
            try:
                with ZipFile(zip_file, 'w') as zipf:
                    zipf.write(self.db.db_path)
                return FileResponse(zip_file, media_type='application/zip', filename=zip_file)
            except Exception as e:
                raise HTTPException(status_code=500, detail=str(e))
            finally:
                background_tasks.add_task(self.delete_file, zip_file)


        @self.app.post("/restore", tags=['Utils'], summary="Restore the database")
        async def restore_database(background_tasks: BackgroundTasks,file: UploadFile = File(...)):
            zip_file = "restored.zip"
            try:
                if os.path.exists(self.db.db_path):
                    os.remove(self.db.db_path)
                with open(zip_file, "wb") as buffer:
                    shutil.copyfileobj(file.file, buffer)
                with ZipFile(zip_file, 'r') as zipf:
                    zipf.extractall()
                
                return {"message": "Database restored successfully."}
            except Exception as e:
                raise HTTPException(status_code=500, detail=str(e))
            finally:
                background_tasks.add_task(self.delete_file, zip_file)


    def delete_file(self,file_path: str):
        try:
            os.remove(file_path)    
        except Exception as e:
            logger.warning(f"Error deleting file: {e}") 

        
    def start(self):
        uvicorn.run(self.app, host="0.0.0.0", port=8082)