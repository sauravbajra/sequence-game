# Sequence Game (Go Backend & Web Client)

## Overview

This project is a multiplayer implementation of the classic board game Sequence. It features a backend built with Go to handle game logic, state management, and real-time communication via WebSockets. The frontend is a web client built with HTML, Tailwind CSS, Alpine.js, and JavaScript, allowing users to interact with the game in their browsers.

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
* **Valid Move Highlighting:** The web client highlights possible valid moves on the board with a light background when a card is selected from the player's hand.
* **Rejoin Support:** Players can refresh or reconnect and will automatically rejoin their game and hand if their browser localStorage is intact.

## Technologies Used

* **Backend:**
    * Go (Golang)
    * Gorilla WebSocket (`github.com/gorilla/websocket`)
* **Frontend:**
    * HTML5
    * Tailwind CSS (via CDN)
    * Alpine.js (via CDN)
    * JavaScript (Vanilla)

## File Structure

```
.
├── main.go             # Core Go backend server logic
├── static/
│   └── index.html      # HTML web client
├── Makefile            # Makefile for building, running, and cleaning the project
└── README.md           # This file: Project overview, setup, and usage
```

## Setup & Running

1.  **Prerequisites:**
    * Ensure Go is installed on your system (version 1.18 or newer recommended).
    * A modern web browser.

2.  **Get Dependencies:**
    Open your terminal in the project's root directory and run:
    ```sh
    go mod tidy
    ```
    This will ensure all dependencies listed in `go.mod` are installed.

3.  **Prepare Client File:**
    Ensure the `static` directory exists in your project root and contains `index.html`. This file is required for the web client.

4.  **Run the Backend Server:**
    You can run the server in two ways:
    - Using Go directly:
      ```sh
      go run main.go
      ```
    - Or, using the Makefile (recommended):
      ```sh
      make run
      ```
    You should see log messages indicating the server has started, typically on port `8008`.

5.  **Access the Game:**
    Open your web browser and navigate to:
    ```
    http://localhost:8008
    ```
    This will load the web client, and you can start creating or joining games.

## Build and Run with Makefile

This project includes a `Makefile` for convenient building and running:

- **Build the server:**
  ```sh
  make build
  ```
  This compiles the Go backend and produces a `sequence-game` binary.

- **Run the server:**
  ```sh
  make run
  ```
  This builds (if needed) and runs the backend server. By default, it serves the web client at [http://localhost:8008](http://localhost:8008).

- **Clean build artifacts:**
  ```sh
  make clean
  ```
  This removes the compiled binary.

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
* **Board Rendering:** Dynamically creates the 10x10 game board based on data from the server.
* **Hand Display:** Shows the current player's cards.
* **Action Handling:**
    * `handleCardInHandClick()`: Manages card selection from the hand.
    * `highlightValidMoves()`: Visually indicates where the selected card can be played (with a light background highlight).
    * `handleBoardCellClick()`: Sends play actions to the server when a board cell is clicked.
    * Handles "Declare Dead Card," "Create Game," "Join Game," and "Start Game" actions.
* **State Synchronization:** Updates the UI based on messages received from the server.
* **Rejoin Logic:** The client automatically attempts to rejoin the previous game and hand after a refresh or reconnect, using localStorage.

## Important Note on Board Layout

The accuracy of the `boardCardDistribution` array in `main.go` is **critical** for the game to function correctly according to standard Sequence rules. This array defines which card corresponds to each space on the board. Ensure it's verified and complete. The `parseCardID` function is designed to handle suffixes (like `_alt`) in this array for representing the second instance of a card.


