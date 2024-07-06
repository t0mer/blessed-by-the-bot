from loguru import logger
import sqlite3

class SqliteConnector:
    def __init__(self):
        self.db_path = "db/data.db"
        self.conn = self.open_connection()
    # Connect to the SQLite database
    def open_connection(self):
        try:
            conn = sqlite3.connect(self.db_path)
            self.conn = conn
            return conn
        except sqlite3.Error as e:
            logger.error(str(e))
            return None

    # Create tables
    def create_tables(self):
        if self.conn is None:
            self.open_connection()
        else:
            try:
                c = self.conn.cursor()
                c.execute('''
                            CREATE TABLE IF NOT EXISTS Languages (
                                LanguageId INTEGER PRIMARY KEY AUTOINCREMENT,
                                Language TEXT NOT NULL UNIQUE
                            )
                            ''')

                c.execute('''
                            CREATE TABLE IF NOT EXISTS Genders (
                                GenderId INTEGER PRIMARY KEY AUTOINCREMENT,
                                Gender TEXT NOT NULL UNIQUE
                            )
                            ''')

                c.execute('''
                            CREATE TABLE IF NOT EXISTS Blesses (
                                BlessId INTEGER PRIMARY KEY AUTOINCREMENT,
                                GenderId INTEGER,
                                LanguageId INTEGER,
                                Bless TEXT NOT NULL,
                                FOREIGN KEY(GenderId) REFERENCES Genders(GenderId),
                                FOREIGN KEY(LanguageId) REFERENCES Languages(LanguageId)
                            )
                            ''')

                c.execute('''
                            CREATE TABLE IF NOT EXISTS Persons (
                                PersonId INTEGER PRIMARY KEY AUTOINCREMENT,
                                FirstName TEXT NOT NULL,
                                LastName TEXT NOT NULL,
                                BirthDate DATE NOT NULL,
                                GenderId INTEGER,
                                LanguageId INTEGER,
                                PhoneNumber TEXT NOT NULL,
                                PreferredHour INTEGER NOT NULL,
                                FOREIGN KEY(GenderId) REFERENCES Genders(GenderId),
                                FOREIGN KEY(LanguageId) REFERENCES Languages(LanguageId)
                            )
                            ''')

                self.conn.commit()
                logger.info("Tables created successfully")
            except sqlite3.Error as e:
                logger.error(str(e))

    def execute_query(self, query, params=()):
        conn = sqlite3.connect('birthdays.db')
        cursor = conn.cursor()
        cursor.execute(query, params)
        conn.commit()
        conn.close()

    # Inserts
    def insert_language(self, language):
        query = 'INSERT INTO Languages (Language) VALUES (?)'
        self.execute_query(query, (language,))

    def insert_gender(self, gender):
        query = 'INSERT INTO Genders (Gender) VALUES (?)'
        self.execute_query(query, (gender,))

    def insert_bless(self, gender_id, language_id, bless):
        query = 'INSERT INTO Blesses (GenderId, LanguageId, Bless) VALUES (?, ?, ?)'
        self.execute_query(query, (gender_id, language_id, bless))

    def insert_person(self, first_name, last_name, birth_date, gender_id, language_id, phone_number, preferred_hour):
        query = 'INSERT INTO Persons (FirstName, LastName, BirthDate, GenderId, LanguageId, PhoneNumber, PreferredHour) VALUES (?, ?, ?, ?, ?, ?, ?)'
        self.execute_query(query, (first_name, last_name, birth_date, gender_id, language_id, phone_number, preferred_hour))

    # Updates

    def update_language(self, language_id, new_language):
        query = 'UPDATE Languages SET Language = ? WHERE LanguageId = ?'
        self.execute_query(query, (new_language, language_id))

    def update_gender(self, gender_id, new_gender):
        query = 'UPDATE Genders SET Gender = ? WHERE GenderId = ?'
        self.execute_query(query, (new_gender, gender_id))

    def update_bless(self, bless_id, gender_id, language_id, bless):
        query = 'UPDATE Blesses SET GenderId = ?, LanguageId = ?, Bless = ? WHERE BlessId = ?'
        self.execute_query(query, (gender_id, language_id, bless, bless_id))

    def update_person(self, person_id, first_name, last_name, birth_date, gender_id, language_id, phone_number, preferred_hour):
        query = 'UPDATE Persons SET FirstName = ?, LastName = ?, BirthDate = ?, GenderId = ?, LanguageId = ?, PhoneNumber = ?, PreferredHour = ? WHERE PersonId = ?'
        self.execute_query(query, (first_name, last_name, birth_date, gender_id, language_id, phone_number, preferred_hour, person_id))


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


# Example usage:
connector = SqliteConnector()

connector.conn.close()
