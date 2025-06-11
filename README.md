# Sequence Game (Go Backend & Web Client)

## Overview

This project is a multiplayer implementation of the classic board game Sequence. It features a backend built with Go to handle game logic, state management, and real-time communication via WebSockets. The frontend is a web client built with HTML, Tailwind CSS, and JavaScript, allowing users to interact with the game in their browsers.

## Features

* **Go Backend:** Manages all game rules, player actions, and board state.
* **WebSocket Communication:** Enables real-time, bidirectional communication between the server and clients.
* **HTML/CSS/JS Web Client:** Provides the user interface for playing the game.
* **Multiplayer Support:** Allows multiple players to join and play a game concurrently.
* **Core Sequence Rules Implemented:**
    * Card dealing and hand management.
    * Placing chips on the board based on played cards.
    * Special actions for Two-Eyed Jacks (wild placement) and One-Eyed Jacks (chip removal).
    * Declaring "dead cards."
    * Detection of sequences (5 in a row).
    * Win condition checking.
* **Static File Serving:** The Go backend also serves the static HTML client.
* **Card Emojis:** Uses card suit emojis for a more visual representation on the board and in player hands.
* **Valid Move Highlighting:** The web client highlights possible valid moves on the board when a card is selected from the player's hand.

## Technologies Used

* **Backend:**
    * Go (Golang)
    * Gorilla WebSocket (`github.com/gorilla/websocket`)
* **Frontend:**
    * HTML5
    * Tailwind CSS (via CDN)
    * JavaScript (Vanilla)

## File Structure


.
├── main.go             # Core Go backend server logic
├── static/
│   └── index.html      # HTML web client
└── README.md           # This file: Project overview, setup, and usage


## Setup & Running

1.  **Prerequisites:**
    * Ensure Go is installed on your system (version 1.18 or newer recommended).
    * A modern web browser.

2.  **Get Dependencies:**
    Open your terminal in the project's root directory and run:
    ```bash
    go get [github.com/gorilla/websocket](https://github.com/gorilla/websocket)
    ```

3.  **Prepare Client File:**
    * Create a directory named `static` in the root of your project.
    * Place the web client HTML file (provided as `index.html` during development) inside this `static` directory.

4.  **Run the Backend Server:**
    Navigate to the project's root directory in your terminal and run:
    ```bash
    go run main.go
    ```
    You should see log messages indicating the server has started, typically on port `8080`.

5.  **Access the Game:**
    Open your web browser and navigate to:
    ```
    http://localhost:8080
    ```
    This will load the web client, and you can start creating or joining games.

## Key Backend Components (`main.go`)

* **Data Structures:**
    * `Card`: Represents a playing card (Rank, Suit, ID, Emoji Display).
    * `Player`: Stores player-specific information (ID, Name, Hand, ChipColor, Connection).
    * `BoardSpace`: Represents a single cell on the game board (Card, OccupiedBy, IsCorner, IsLocked).
    * `Game`: Encapsulates the entire game state (Board, Players, DrawPile, CurrentTurn, etc.).
* **Game Logic:**
    * `NewGame()`: Initializes a new game instance.
    * `initializeBoardLayout()`: Sets up the board using `boardCardDistribution`. **Crucial for correct gameplay.**
    * `parseCardID()`: Converts string representations from `boardCardDistribution` into `Card` objects.
    * `AddPlayer()`, `StartGame()`: Manage player joining and game start.
    * `PlayAction()`, `HandleDeadCard()`: Process player moves.
    * `checkForSequencesAfterPlay()`: Detects completed sequences.
* **WebSocket Handling (`handleWebSocket`):** Manages client connections, message routing, and game state broadcasts.
* **Static File Serving (`serveClient`):** Serves the `index.html` client.

## Key Frontend Components (`static/index.html`)

* **WebSocket Connection:** Establishes and maintains communication with the Go backend.
* **UI Management:**
    * Game setup section (create/join game, player name).
    * Game area display (board, player info, hand).
* **Board Rendering (`renderBoard`):** Dynamically creates the 10x10 game board based on data from the server.
* **Hand Display (`updatePlayerHand`):** Shows the current player's cards.
* **Action Handling:**
    * `handleCardInHandClick()`: Manages card selection from the hand.
    * `highlightValidMoves()`: Visually indicates where the selected card can be played.
    * `handleBoardCellClick()`: Sends play actions to the server when a board cell is clicked.
    * Handles "Declare Dead Card," "Create Game," "Join Game," and "Start Game" actions.
* **State Synchronization:** Updates the UI based on messages received from the server.

## Important Note on Board Layout

The accuracy of the `boardCardDistribution` array in `main.go` is **critical** for the game to function correctly according to standard Sequence rules. This array defines which card corresponds to each space on the board. Ensure it's verified and complete. The `parseCardID` function is designed to handle suffixes (like `_alt`) in this array for representing the second instance of a card.

---


