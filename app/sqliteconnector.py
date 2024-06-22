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
            return conn
        except sqlite3.Error as e:
            logger.error(str(e))
            return None

    # Create tables
    def create_tables(self):
        if self.conn is not None:
            try:
                c = self.conn.cursor()
                c.execute('''CREATE TABLE IF NOT EXISTS Users (
                                Id INTEGER PRIMARY KEY,
                                FirstName TEXT NOT NULL,
                                LastName TEXT NOT NULL,
                                GenderId INTEGER,
                                Birthdate DATE,
                                WhatsappNumber TEXT,
                                LanguageId INTEGER,
                                IsVip BOOLEAN DEFAULT 0,
                                FOREIGN KEY (GenderId) REFERENCES Genders(Id),
                                FOREIGN KEY (LanguageId) REFERENCES Languages(Id)
                            )''')

                c.execute('''CREATE TABLE IF NOT EXISTS Blesses (
                                BlessId INTEGER PRIMARY KEY,
                                LanguageId INTEGER,
                                GenderId INTEGER,
                                BlessText TEXT,
                                FOREIGN KEY (LanguageId) REFERENCES Languages(Id),
                                FOREIGN KEY (GenderId) REFERENCES Genders(Id)
                            )''')

                c.execute('''CREATE TABLE IF NOT EXISTS Genders (
                                GenderId INTEGER PRIMARY KEY,
                                GenderName TEXT UNIQUE
                            )''')

                c.execute('''CREATE TABLE IF NOT EXISTS Languages (
                                LanguageId INTEGER PRIMARY KEY,
                                LanguageName TEXT UNIQUE
                            )''')

                self.conn.commit()
                logger.info("Tables created successfully")
            except sqlite3.Error as e:
                logger.error(str(e))
        else:
            logger.error("Failed to connect to database")

# Insert into Users table
    def insert_user(self, first_name, last_name, gender_id, birthdate, whatsapp_number, language_id, is_vip=False):
        try:
            c = self.conn.cursor()
            c.execute('''INSERT INTO Users (FirstName, LastName, GenderId, Birthdate, WhatsappNumber, LanguageId, IsVip)
                        VALUES (?, ?, ?, ?, ?, ?, ?)''',
                    (first_name, last_name, gender_id, birthdate, whatsapp_number, language_id, is_vip))
            self.conn.commit()
            logger.info("User inserted successfully")
        except sqlite3.Error as e:
            logger.error(str(e))

    # Update Users table
    def update_user(self, user_id, first_name=None, last_name=None, gender_id=None, birthdate=None,
                    whatsapp_number=None, language_id=None, is_vip=None):
        try:
            c = self.conn.cursor()
            update_query = "UPDATE Users SET "
            params = []

            if first_name:
                update_query += "FirstName=?, "
                params.append(first_name)
            if last_name:
                update_query += "LastName=?, "
                params.append(last_name)
            if gender_id:
                update_query += "GenderId=?, "
                params.append(gender_id)
            if birthdate:
                update_query += "Birthdate=?, "
                params.append(birthdate)
            if whatsapp_number:
                update_query += "WhatsappNumber=?, "
                params.append(whatsapp_number)
            if language_id:
                update_query += "LanguageId=?, "
                params.append(language_id)
            if is_vip is not None:
                update_query += "IsVip=?, "
                params.append(is_vip)

            # Remove the trailing comma and space
            update_query = update_query[:-2]
            update_query += " WHERE Id=?"

            # Append user_id as the last parameter
            params.append(user_id)

            c.execute(update_query, tuple(params))
            self.conn.commit()
            logger.info("User updated successfully")
        except sqlite3.Error as e:
            logger.error((e))

    # Delete from Users table
    def delete_user(self, user_id):
        try:
            c = self.conn.cursor()
            c.execute("DELETE FROM Users WHERE Id=?", (user_id,))
            self.conn.commit()
            logger.info("User deleted successfully")
        except sqlite3.Error as e:
            logger.error((e))

    # Select from Users table
    def select_users(self):
        try:
            c = self.conn.cursor()
            c.execute("SELECT * FROM Users")
            rows = c.fetchall()
            return rows
        except sqlite3.Error as e:
            logger.error(str(e))
            
    # Select from Users table
    def select_languages(self):
        try:
            c = self.conn.cursor()
            c.execute("SELECT * FROM Languages")
            rows = c.fetchall()
            return rows
        except sqlite3.Error as e:
            logger.error(str(e))


    # Select from Users table
    def select_genders(self):
        try:
            c = self.conn.cursor()
            c.execute("SELECT * FROM Genders")
            rows = c.fetchall()
            return rows
        except sqlite3.Error as e:
            logger.error(str(e))


    # Insert into Genders table
    def insert_gender(self, gender_name):
        try:
            c = self.conn.cursor()
            c.execute('''INSERT INTO Genders (GenderName) VALUES (?)''', (gender_name,))
            self.conn.commit()
            logger.info("Gender inserted successfully")
        except sqlite3.Error as e:
            logger.error((e))

    # Insert into Languages table
    def insert_language(self, language_name):
        try:
            c = self.conn.cursor()
            c.execute('''INSERT INTO Languages (LanguageName) VALUES (?)''', (language_name,))
            self.conn.commit()
            print("Language inserted successfully")
        except sqlite3.Error as e:
            logger.error(str(e))

    # Insert into Blesses table
    def insert_bless(self, language_id, gender_id, bless_text):
        try:
            c = self.conn.cursor()
            c.execute('''INSERT INTO Blesses (LanguageId, GenderId, BlessText) VALUES (?, ?, ?)''',
                    (language_id, gender_id, bless_text))
            self.conn.commit()
            logger.info("Bless inserted successfully")
        except sqlite3.Error as e:
            logger.error((e))

# Example usage:
connector = SqliteConnector()

# Inserting data
connector.insert_user("John", "Doe", 1, "1990-01-01", "123456789", 1)
connector.insert_gender("Zebra")
connector.insert_language("Polish")
connector.insert_bless(1, 1, "May your day be blessed with happiness.")

print(connector.select_users())
print(connector.select_languages())
print(connector.select_genders())

# Don't forget to close the connection when done
connector.conn.close()
