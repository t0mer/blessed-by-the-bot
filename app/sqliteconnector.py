from loguru import logger
import sqlite3

class SqliteConnector:
    def __init__(self):
        self.db_path = "db/data.db"
        self.conn = None
    # Connect to the SQLite database
    def open_connection(self):
        if self.conn is None:
            try:
                self.conn = sqlite3.connect(self.db_path)
                logger.info("Connection opened successfully.")
            except sqlite3.Error as e:
                logger.error(f"Error connecting to database: {e}")
        else:
            logger.debug("Connection is already open.")


    def close_connection(self):
        if self.conn is not None:
            self.conn.close()
            self.conn = None
            logger.info("Connection closed.")
        else:
            logger.info("No connection to close.")


    # Create tables
    def create_tables(self):
        try:
            self.open_connection()
            cursor = self.conn.cursor()
            cursor.execute('''
                        CREATE TABLE IF NOT EXISTS Languages (
                            LanguageId INTEGER PRIMARY KEY AUTOINCREMENT,
                            Language TEXT NOT NULL UNIQUE
                        )
                        ''')

            cursor.execute('''
                        CREATE TABLE IF NOT EXISTS Genders (
                            GenderId INTEGER PRIMARY KEY AUTOINCREMENT,
                            Gender TEXT NOT NULL UNIQUE
                        )
                        ''')

            cursor.execute('''
                        CREATE TABLE IF NOT EXISTS Blesses (
                            BlessId INTEGER PRIMARY KEY AUTOINCREMENT,
                            GenderId INTEGER,
                            LanguageId INTEGER,
                            Bless TEXT NOT NULL,
                            FOREIGN KEY(GenderId) REFERENCES Genders(GenderId),
                            FOREIGN KEY(LanguageId) REFERENCES Languages(LanguageId)
                        )
                        ''')

            cursor.execute('''
                        CREATE TABLE IF NOT EXISTS Persons (
                            PersonId INTEGER PRIMARY KEY AUTOINCREMENT,
                            FirstName TEXT NOT NULL,
                            LastName TEXT NOT NULL,
                            BirthDate DATE NOT NULL,
                            GenderId INTEGER,
                            LanguageId INTEGER,
                            PhoneNumber TEXT NOT NULL,
                            PreferredHour INTEGER NOT NULL,
                            Intro TEXT NOT NULL,
                            FOREIGN KEY(GenderId) REFERENCES Genders(GenderId),
                            FOREIGN KEY(LanguageId) REFERENCES Languages(LanguageId)
                        )
                        ''')

            self.conn.commit()
            self.close_connection()
            logger.info("Tables created successfully")
        except sqlite3.Error as e:
            logger.error(str(e))

    def execute_query(self, query, params=(),is_insert=False):
        self.open_connection()
        cursor = self.conn.cursor()
        cursor.execute(query, params)
        self.conn.commit()
        if is_insert:
            lastrowid = cursor.lastrowid
            self.close_connection()
            return lastrowid
        self.close_connection()
            

    # Inserts
    def insert_language(self, language):
        query = 'INSERT INTO Languages (Language) VALUES (?)'
        return self.execute_query(query, (language,),True)

    def insert_gender(self, gender):
        query = 'INSERT INTO Genders (Gender) VALUES (?)'
        return self.execute_query(query, (gender,),True)

    def insert_bless(self, gender_id, language_id, bless):
        query = 'INSERT INTO Blesses (GenderId, LanguageId, Bless) VALUES (?, ?, ?)'
        return self.execute_query(query, (gender_id, language_id, bless),True)

    def insert_person(self, first_name, last_name, birth_date, gender_id, language_id, phone_number, preferred_hour, intro):
        query = 'INSERT INTO Persons (FirstName, LastName, BirthDate, GenderId, LanguageId, PhoneNumber, PreferredHour, intro) VALUES (?, ?, ?, ?, ?, ?, ?, ?)'
        return self.execute_query(query, (first_name, last_name, birth_date, gender_id, language_id, phone_number, preferred_hour, intro),True)

    def insert_configurarion(self,whatsapp_api_url, whatsapp_api_token, whatsapp_api_session_name):
        query = 'INSERT INTO Configuration (ConfigId, WhatsappApiUrl, WhatsappApiToken, WhatsappApiSessionName) VALUES (1, ?, ?, ?)'''
        return self.execute_query(query, (whatsapp_api_url, whatsapp_api_token, whatsapp_api_session_name),True)
        
    
    
    
    # Updates

    def update_language(self, language_id, new_language):
        query = 'UPDATE Languages SET Language = ? WHERE LanguageId = ?'
        self.execute_query(query, (new_language, language_id))

    def update_gender(self, gender_id, new_gender):
        query = 'UPDATE Genders SET Gender = ? WHERE GenderId = ?'
        self.execute_query(query, (new_gender, gender_id))

    def update_bless(self, bless_id, gender_id=None, language_id=None, bless=None):
        query = 'UPDATE Blesses SET '
        params = []
        if gender_id is not None:
            query += 'GenderId=?, '
            params.append(gender_id)
        if language_id is not None:
            query += 'LanguageId=?, '
            params.append(language_id)
        if bless:
            query += 'Bless=?, '
            params.append(bless)
        query = query.rstrip(', ') + ' WHERE BlessId=?'
        params.append(bless_id)
        self.execute_query(query, params)

    def update_person(self, person_id, first_name=None, last_name=None, birth_date=None, gender_id=None, language_id=None, phone_number=None, preferred_hour=None, intro=None):
        query = 'UPDATE Persons SET '
        params = []
        if first_name:
            query += 'FirstName=?, '
            params.append(first_name)
        if last_name:
            query += 'LastName=?, '
            params.append(last_name)
        if birth_date:
            query += 'BirthDate=?, '
            params.append(birth_date)
        if gender_id is not None:
            query += 'GenderId=?, '
            params.append(gender_id)
        if language_id is not None:
            query += 'LanguageId=?, '
            params.append(language_id)
        if phone_number:
            query += 'PhoneNumber=?, '
            params.append(phone_number)
        if preferred_hour is not None:
            query += 'PreferredHour=?, '
            params.append(preferred_hour)
        if intro:
            query += 'Intro=?, '
            params.append(intro)
        query = query.rstrip(', ') + ' WHERE PersonId=?'
        params.append(person_id)
        self.execute_query(query, params)


    def update_configuration(self, whatsapp_api_url=None, whatsapp_api_token=None, whatsapp_api_session_name=None):
        try:
            update_query = "UPDATE Configuration SET "
            params = []

            if whatsapp_api_url:
                update_query += "WhatsappApiUrl=?, "
                params.append(whatsapp_api_url)
            if whatsapp_api_token:
                update_query += "WhatsappApiToken=?, "
                params.append(whatsapp_api_token)
            if whatsapp_api_session_name:
                update_query += "WhatsappApiSessionName=?, "
                params.append(whatsapp_api_session_name)

            # Remove the trailing comma and space
            update_query = update_query[:-2]
            update_query += " WHERE ConfigId=1"
            self.execute_query(update_query, params)
            return {"message": "Configuration updated successfully."}
        except Exception as e:
            self.conn.rollback()
            logger.error(f"Error updating configuration. {e}")



    # Deletes
    
    def delete_language(self, language_id):
        query = 'DELETE FROM Languages WHERE LanguageId = ?'
        self.execute_query(query, (language_id,))

    def delete_gender(self, gender_id):
        query = 'DELETE FROM Genders WHERE GenderId = ?'
        self.execute_query(query, (gender_id,))

    def delete_bless(self, bless_id):
        query = 'DELETE FROM Blesses WHERE BlessId = ?'
        self.execute_query(query, (bless_id,))

    def delete_person(self, person_id):
        query = 'DELETE FROM Persons WHERE PersonId = ?'
        self.execute_query(query, (person_id,))

    def delete_configuration(self):
        try:
            self.execute_query("DELETE FROM Configuration WHERE ConfigId=1")
            return {"message": "Configuration deleted successfully."}
        except Exception as e:
            self.conn.rollback()
            logger.error(f"Error deleting configuration. {e}")



    # Selects
    
    def select_all_languages(self, api_call=False):
        try:
            self.open_connection()
            cursor = self.conn.cursor()
            cursor.execute('SELECT LanguageId, Language FROM Languages')
            if api_call == True:
                rows = [dict((cursor.description[i][0], value) \
                for i, value in enumerate(row)) for row in cursor.fetchall()]
                cursor.close()      
                return (rows[0] if rows else None) if False else rows
            else:
                rows = cursor.fetchall()
                cursor.close()
            return rows
        finally:
            self.close_connection()
            

    def select_all_genders(self, api_call=False):
        try:
            self.open_connection()
            cursor = self.conn.cursor()
            cursor.execute('SELECT GenderId, Gender FROM Genders')
            if api_call == True:
                rows = [dict((cursor.description[i][0], value) \
                for i, value in enumerate(row)) for row in cursor.fetchall()]
                cursor.close()      
                return (rows[0] if rows else None) if False else rows
            else:
                rows = cursor.fetchall()
                cursor.close()
            return rows
        finally:
            self.close_connection()
            
            
    def select_all_blesses(self, api_call=False):
        try:
            self.open_connection()
            cursor = self.conn.cursor()
            cursor.execute('SELECT BlessId, GenderId, LanguageId, Bless FROM Blesses')
            if api_call == True:
                rows = [dict((cursor.description[i][0], value) \
                for i, value in enumerate(row)) for row in cursor.fetchall()]
                cursor.close()      
                return (rows[0] if rows else None) if False else rows
            else:
                rows = cursor.fetchall()
                cursor.close()
            return rows
        finally:
            self.close_connection()
            
    def select_all_persons(self, api_call=False):
        try:
            self.open_connection()
            cursor = self.conn.cursor()
            cursor.execute('SELECT PersonId, FirstName, LastName, BirthDate, GenderId, LanguageId, PhoneNumber, PreferredHour, Intro FROM Persons')
            if api_call == True:
                rows = [dict((cursor.description[i][0], value) \
                for i, value in enumerate(row)) for row in cursor.fetchall()]
                cursor.close()      
                return (rows[0] if rows else None) if False else rows
            else:
                rows = cursor.fetchall()
                cursor.close()
            return rows
        finally:
            self.close_connection()

    def get_configuration(self, api_call=False):
        try:
            self.open_connection()
            cursor = self.conn.cursor()
            cursor.execute("SELECT ConfigId, WhatsappApiUrl, WhatsappApiToken, WhatsappApiSessionName FROM Configuration WHERE ConfigId=1")
            if api_call == True:
                rows = [dict((cursor.description[i][0], value) \
                for i, value in enumerate(row)) for row in cursor.fetchall()]
                cursor.close()      
                return (rows[0] if rows else None) if False else rows
            else:
                rows = cursor.fetchall()
                cursor.close()
            return rows
        finally:
            self.close_connection()


# Example usage:
connector = SqliteConnector()
connector.create_tables()
connector.select_all_persons()


