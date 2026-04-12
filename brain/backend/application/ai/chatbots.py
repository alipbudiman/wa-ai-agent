

class Chatbot:
    def __init__(self, name):
        self.name = name

    def respond(self, message):
        return f"{self.name} says: {message}"