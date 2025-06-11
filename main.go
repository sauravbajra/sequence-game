package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"math/big" // For crypto/rand
	"net/http"
	"os"            // Added for checking file existence
	"path/filepath" // Added for path manipulation
	"strconv"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

// --- Constants & Configuration ---
const (
	BoardSize             = 10
	NumDecks              = 2
	DefaultSequencesToWin = 2
	StaticDir             = "./static"   // Directory for static files
	ClientHTMLFile        = "index.html" // Name of your HTML client file
)

// --- Enums for Cards ---
type Suit int

const (
	Hearts Suit = iota
	Diamonds
	Clubs
	Spades
	NoSuit // For Jokers or special cards if extended
)

// String for internal ID construction (e.g., "H", "S")
func (s Suit) String() string {
	return []string{"H", "D", "C", "S", "X"}[s]
}

// ToEmoji for display
func (s Suit) ToEmoji() string {
	return map[Suit]string{Hearts: "♥️", Diamonds: "♦️", Clubs: "♣️", Spades: "♠️", NoSuit: ""}[s]
}

type Rank int

const (
	Ace Rank = iota + 1 // Ace as 1 for simplicity in loops, can map to 'A'
	Two
	Three
	Four
	Five
	Six
	Seven
	Eight
	Nine
	Ten   // Numeric 10
	Jack  // J
	Queen // Q
	King  // K
	NoRank
)

// String for internal ID construction (e.g., "A", "10", "K")
func (r Rank) String() string {
	if r >= Two && r <= Ten {
		return strconv.Itoa(int(r)) // "2", "3", ..., "10"
	}
	return map[Rank]string{
		Ace:   "A",
		Jack:  "J",
		Queen: "Q",
		King:  "K",
	}[r]
}

// ToUnicode for display part of emoji string
func (r Rank) ToUnicode() string {
	// Same as String() for ranks, but explicit for display purposes
	if r >= Two && r <= Ten {
		return strconv.Itoa(int(r))
	}
	return map[Rank]string{Ace: "A", Jack: "J", Queen: "Q", King: "K"}[r]
}

// --- Core Data Structures ---

// Card represents a playing card
type Card struct {
	Rank Rank
	Suit Suit
	ID   string // e.g., "KH" for King of Hearts, "10S" for 10 of Spades
}

// ToEmojiString creates a display string like "A♠️"
func (c Card) ToEmojiString() string {
	if c.Rank == NoRank || c.Suit == NoSuit {
		return c.ID // Fallback for special cases or if ID is already display-ready
	}
	return c.Rank.ToUnicode() + c.Suit.ToEmoji()
}

// Player represents a player in the game
type Player struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Hand        []Card          `json:"-"`                     // Hide hand from other players in general broadcasts
	VisibleHand []string        `json:"visibleHand,omitempty"` // For the player themselves (contains Card.ID)
	ChipColor   string          `json:"chipColor"`
	Sequences   int             `json:"sequences"`
	Conn        *websocket.Conn `json:"-"` // WebSocket connection
	IsConnected bool            `json:"isConnected"`
}

// BoardSpace represents a single space on the game board
type BoardSpace struct {
	Card         *Card  `json:"card,omitempty"` // The card printed on this space (nil for corners)
	OccupiedBy   string `json:"occupiedBy"`     // PlayerID of the chip on this space, or "" if empty
	IsCorner     bool   `json:"isCorner"`       // To mark the free corner spaces
	IsLocked     bool   `json:"isLocked"`       // If part of a completed sequence
	DisplayValue string `json:"displayValue"`   // e.g. "A♠️", "7♦️", "FREE"
}

// Position represents a coordinate on the board
type Position struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// Game represents the entire game state
type Game struct {
	ID                string                           `json:"id"`
	Board             [BoardSize][BoardSize]BoardSpace `json:"board"`
	Players           map[string]*Player               `json:"players"`     // Map PlayerID to Player struct
	PlayerOrder       []string                         `json:"playerOrder"` // To maintain turn order
	CurrentTurnIndex  int                              `json:"currentTurnIndex"`
	DrawPile          []Card                           `json:"-"` // Not usually sent to client
	DrawPileCount     int                              `json:"drawPileCount"`
	DiscardPile       []Card                           `json:"-"`
	GamePhase         string                           `json:"gamePhase"`        // e.g., "Lobby", "InProgress", "Finished"
	Winner            string                           `json:"winner,omitempty"` // PlayerID or TeamID
	NumSequencesToWin int                              `json:"numSequencesToWin"`
	MaxPlayers        int                              `json:"maxPlayers"`
	HostID            string                           `json:"hostId"`
	mu                sync.Mutex
}

// --- Game Management ---
var (
	games    = make(map[string]*Game)
	gamesMu  sync.Mutex
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(r *http.Request) bool { return true }, // Allow all origins for simplicity
	}
)

// generateID creates a unique random ID
func generateID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "errorID" // Fallback
	}
	return hex.EncodeToString(bytes)
}

// --- Card and Deck Logic ---

// newDeck creates a specified number of standard 52-card decks
func newDeck(numDecks int) []Card {
	var deck []Card
	suits := []Suit{Hearts, Diamonds, Clubs, Spades}
	ranks := []Rank{Ace, Two, Three, Four, Five, Six, Seven, Eight, Nine, Ten, Jack, Queen, King}

	for i := 0; i < numDecks; i++ {
		for _, suit := range suits {
			for _, rank := range ranks {
				// Card ID is canonical, e.g., "AS", "10H", "KD"
				cardID := rank.String() + suit.String()
				deck = append(deck, Card{Rank: rank, Suit: suit, ID: cardID})
			}
		}
	}
	return deck
}

// shuffleDeck shuffles a slice of cards
func shuffleDeck(deck []Card) {
	n := len(deck)
	for i := n - 1; i > 0; i-- {
		jBig, _ := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		j := jBig.Int64()
		deck[i], deck[j] = deck[j], deck[i]
	}
}

// dealCards deals cards to players based on game rules
func (g *Game) dealCards() {
	cardsPerPlayer := 0
	numPlayers := len(g.Players)

	switch {
	case numPlayers <= 2:
		cardsPerPlayer = 7
	case numPlayers <= 4:
		cardsPerPlayer = 6
	case numPlayers <= 6:
		cardsPerPlayer = 5
	case numPlayers <= 9:
		cardsPerPlayer = 4
	case numPlayers <= 12:
		cardsPerPlayer = 3
	default:
		log.Printf("Warning: Too many players (%d) for standard dealing.", numPlayers)
		cardsPerPlayer = 3
	}

	for i := 0; i < cardsPerPlayer; i++ {
		for _, playerID := range g.PlayerOrder {
			player := g.Players[playerID]
			if len(g.DrawPile) > 0 {
				card := g.DrawPile[0]
				g.DrawPile = g.DrawPile[1:]
				player.Hand = append(player.Hand, card)
			}
		}
	}
	g.DrawPileCount = len(g.DrawPile)
}

// drawCard allows a player to draw a card
func (g *Game) drawCard(playerID string) (*Card, error) {
	player, ok := g.Players[playerID]
	if !ok {
		return nil, fmt.Errorf("player %s not found", playerID)
	}
	if len(g.DrawPile) == 0 {
		return nil, fmt.Errorf("draw pile is empty")
	}

	card := g.DrawPile[0]
	g.DrawPile = g.DrawPile[1:]
	g.DrawPileCount = len(g.DrawPile)
	player.Hand = append(player.Hand, card)
	return &card, nil
}

// --- Board Initialization ---

// Standard Sequence Game Board Layout.
// Each of the 48 non-Jack cards (A, K, Q, 10, 9, 8, 7, 6, 5, 4, 3, 2 for each suit) appears twice.
// "FREE" denotes a corner space. Jacks are not on the board.
// Card IDs are "RankSuit", e.g., "AS", "10D".
// Suffixes like "_alt" are used in this array literal for the second instance of a card
// to make the string unique if needed for visual clarity or specific tools;
// parseCardID function will strip these suffixes to get the canonical card ID.
var boardCardDistribution = [BoardSize][BoardSize]string{
	{"FREE", "2S", "3S", "4S", "5S", "6S", "7S", "8S", "9S", "FREE"},
	{"6C", "7C", "8C", "9C", "10C", "QC", "KC", "AC", "AD", "10S"},
	{"5C", "4C", "3C", "2C", "AH", "KH", "QH", "10H", "KD", "AS"},
	{"4D", "5D", "6D", "7D", "8D", "9D", "10D", "QD", "KS", "2D"},
	{"3D", "2D_alt", "AS_alt", "KS_alt", "QS", "10S_alt", "9S_alt", "8S_alt", "QS_alt", "2H"},
	{"4H", "5H", "6H", "7H", "8H", "9H", "10H_alt", "QH_alt", "KH_alt", "3H"},
	{"3S_alt", "2H_alt", "AH_alt", "AC_alt", "KC_alt", "QC_alt", "10C_alt", "9C_alt", "8C_alt", "4S_alt"},
	{"2S_alt", "3H_alt", "4H_alt", "5H_alt", "6H_alt", "7H_alt", "8H_alt", "9H_alt", "7C_alt", "5S_alt"},
	{"AD_alt", "KD_alt", "QD_alt", "10D_alt", "9D_alt", "8D_alt", "7D_alt", "6D_alt", "6C_alt", "6S_alt"},
	{"FREE", "5C_alt", "4C_alt", "3C_alt", "2C_alt", "4D_alt", "5D_alt", "7S_alt", "9S_another", "FREE"}, // Corrected last row for standard layout
}

// parseCardID parses a string like "AS" or "10D_alt" into a Card struct.
// It expects canonical IDs (e.g., "10S" not "TS").
func parseCardID(idStr string) (*Card, error) {
	if len(idStr) < 2 {
		return nil, fmt.Errorf("card ID too short: %s", idStr)
	}

	// Normalize by removing potential suffixes like "_alt", "_another", etc.
	normalizedIDStr := idStr
	if strings.Contains(idStr, "_") {
		parts := strings.Split(idStr, "_")
		normalizedIDStr = parts[0] // Take the part before the first underscore
	}

	rankStr := ""
	suitChar := ""

	// Handle "10" rank
	if strings.HasPrefix(normalizedIDStr, "10") {
		if len(normalizedIDStr) != 3 {
			return nil, fmt.Errorf("invalid card ID for 10: %s (from %s)", normalizedIDStr, idStr)
		}
		rankStr = "10"
		suitChar = string(normalizedIDStr[2])
	} else {
		if len(normalizedIDStr) != 2 {
			return nil, fmt.Errorf("invalid card ID format: %s (from %s)", normalizedIDStr, idStr)
		}
		rankStr = string(normalizedIDStr[0])
		suitChar = string(normalizedIDStr[1])
	}

	var rank Rank
	switch rankStr {
	case "A":
		rank = Ace
	case "2":
		rank = Two
	case "3":
		rank = Three
	case "4":
		rank = Four
	case "5":
		rank = Five
	case "6":
		rank = Six
	case "7":
		rank = Seven
	case "8":
		rank = Eight
	case "9":
		rank = Nine
	case "10":
		rank = Ten
	// Jacks are not on the board, so no case for "J" from board layout
	case "Q":
		rank = Queen
	case "K":
		rank = King
	default:
		return nil, fmt.Errorf("unknown rank string: '%s' in ID '%s' (from %s)", rankStr, normalizedIDStr, idStr)
	}

	var suit Suit
	switch suitChar {
	case "S":
		suit = Spades
	case "H":
		suit = Hearts
	case "D":
		suit = Diamonds
	case "C":
		suit = Clubs
	default:
		return nil, fmt.Errorf("unknown suit char: '%s' in ID '%s' (from %s)", suitChar, normalizedIDStr, idStr)
	}

	// Construct canonical ID to ensure consistency
	canonicalID := rank.String() + suit.String()
	return &Card{Rank: rank, Suit: suit, ID: canonicalID}, nil
}

func (g *Game) initializeBoardLayout() {
	for r := 0; r < BoardSize; r++ {
		for c := 0; c < BoardSize; c++ {
			idOnBoard := boardCardDistribution[r][c]
			if idOnBoard == "FREE" {
				g.Board[r][c] = BoardSpace{IsCorner: true, OccupiedBy: "CORNER", DisplayValue: "FREE"}
			} else if idOnBoard == "EMPTY" || idOnBoard == "" { // Should not happen with complete board
				g.Board[r][c] = BoardSpace{DisplayValue: " "}
			} else {
				card, err := parseCardID(idOnBoard)
				if err != nil {
					log.Printf("Error parsing card ID '%s' for board at [%d][%d]: %v. Setting as 'ERR'.", idOnBoard, r, c, err)
					g.Board[r][c] = BoardSpace{DisplayValue: "ERR"}
					continue
				}
				g.Board[r][c] = BoardSpace{Card: card, DisplayValue: card.ToEmojiString()}
			}
		}
	}
	log.Println("Board initialized with a standard Sequence board layout.")
}

// --- Game Actions & Logic ---

// NewGame creates a new game instance
func NewGame(hostID, hostName string, maxPlayers, sequencesToWin int) *Game {
	gameID := generateID()
	if sequencesToWin <= 0 {
		sequencesToWin = DefaultSequencesToWin
	}
	if maxPlayers <= 0 || maxPlayers > 12 {
		maxPlayers = 4
	}

	g := &Game{
		ID: gameID, Players: make(map[string]*Player), DrawPile: newDeck(NumDecks),
		GamePhase: "Lobby", NumSequencesToWin: sequencesToWin, MaxPlayers: maxPlayers,
		HostID: hostID, CurrentTurnIndex: 0,
	}
	g.initializeBoardLayout()
	shuffleDeck(g.DrawPile)
	g.DrawPileCount = len(g.DrawPile)
	log.Printf("New game created: %s by %s", gameID, hostName)
	return g
}

// AddPlayer adds a player to the game
func (g *Game) AddPlayer(playerID, playerName string, conn *websocket.Conn) (*Player, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.GamePhase != "Lobby" {
		return nil, fmt.Errorf("game %s is already in progress", g.ID)
	}
	if len(g.Players) >= g.MaxPlayers {
		return nil, fmt.Errorf("game %s is full", g.ID)
	}

	if existingPlayer, exists := g.Players[playerID]; exists {
		existingPlayer.Conn = conn
		existingPlayer.IsConnected = true
		existingPlayer.Name = playerName
		log.Printf("Player %s (%s) reconnected to game %s", playerName, playerID, g.ID)
		return existingPlayer, nil
	}

	colors := []string{"red", "blue", "green", "yellow", "purple", "orange", "pink", "cyan", "lime", "brown", "teal", "magenta"}
	chipColor := colors[len(g.Players)%len(colors)]

	player := &Player{
		ID: playerID, Name: playerName, ChipColor: chipColor, Conn: conn,
		IsConnected: true, Hand: make([]Card, 0),
	}
	g.Players[playerID] = player
	g.PlayerOrder = append(g.PlayerOrder, playerID)
	log.Printf("Player %s (%s) added to game %s with color %s", playerName, playerID, g.ID, chipColor)
	return player, nil
}

// StartGame transitions the game from Lobby to InProgress
func (g *Game) StartGame(playerID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.HostID != playerID {
		return fmt.Errorf("only the host can start the game")
	}
	if g.GamePhase != "Lobby" {
		return fmt.Errorf("game %s is not in lobby phase", g.ID)
	}
	if len(g.Players) < 2 {
		return fmt.Errorf("not enough players to start. Need at least 2, have %d", len(g.Players))
	}

	g.dealCards()
	g.GamePhase = "InProgress"
	g.CurrentTurnIndex = 0
	log.Printf("Game %s started by %s", g.ID, playerID)
	return nil
}

// removeCardFromHand removes a specific card (by Card.ID) from a player's hand
func (p *Player) removeCardFromHand(cardID string) bool {
	for i, cardInHand := range p.Hand {
		if cardInHand.ID == cardID {
			p.Hand = append(p.Hand[:i], p.Hand[i+1:]...)
			return true
		}
	}
	return false
}

// GetCardFromHand retrieves a card from hand by Card.ID
func (p *Player) GetCardFromHand(cardID string) (*Card, bool) {
	for i := range p.Hand {
		if p.Hand[i].ID == cardID {
			return &p.Hand[i], true
		}
	}
	return nil, false
}

// PlayAction handles a player's move
func (g *Game) PlayAction(playerID string, action PlayerAction) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.GamePhase != "InProgress" {
		return fmt.Errorf("game is not in progress")
	}
	if len(g.PlayerOrder) == 0 || g.CurrentTurnIndex >= len(g.PlayerOrder) || g.PlayerOrder[g.CurrentTurnIndex] != playerID {
		return fmt.Errorf("it's not player %s's turn", playerID)
	}

	player, ok := g.Players[playerID]
	if !ok {
		return fmt.Errorf("player %s not found", playerID)
	}

	playedCard, hasCard := player.GetCardFromHand(action.CardID)
	if !hasCard {
		return fmt.Errorf("player %s does not have card %s", playerID, action.CardID)
	}

	if action.BoardPos.X < 0 || action.BoardPos.X >= BoardSize || action.BoardPos.Y < 0 || action.BoardPos.Y >= BoardSize {
		return fmt.Errorf("invalid board position")
	}
	targetSpace := &g.Board[action.BoardPos.X][action.BoardPos.Y]

	isTwoEyedJack := playedCard.Rank == Jack && (playedCard.Suit == Hearts || playedCard.Suit == Diamonds)
	isOneEyedJack := playedCard.Rank == Jack && (playedCard.Suit == Clubs || playedCard.Suit == Spades)

	if isOneEyedJack {
		if targetSpace.OccupiedBy == "" || targetSpace.OccupiedBy == "CORNER" {
			return fmt.Errorf("cannot remove chip from empty or corner space")
		}
		if targetSpace.OccupiedBy == player.ID {
			return fmt.Errorf("cannot remove your own chip with One-Eyed Jack")
		}
		if targetSpace.IsLocked {
			return fmt.Errorf("cannot remove chip from a locked sequence")
		}

		log.Printf("Player %s uses One-Eyed Jack %s to remove chip at (%d,%d) by %s", player.Name, playedCard.ToEmojiString(), action.BoardPos.X, action.BoardPos.Y, targetSpace.OccupiedBy)
		targetSpace.OccupiedBy = ""
	} else {
		if targetSpace.OccupiedBy != "" && targetSpace.OccupiedBy != "CORNER" {
			return fmt.Errorf("space (%d,%d) is already occupied by %s", action.BoardPos.X, action.BoardPos.Y, targetSpace.OccupiedBy)
		}
		if !isTwoEyedJack {
			if targetSpace.Card == nil {
				return fmt.Errorf("board space (%d,%d) has no card defined, cannot play %s", action.BoardPos.X, action.BoardPos.Y, playedCard.ToEmojiString())
			}
			if targetSpace.Card.ID != playedCard.ID {
				return fmt.Errorf("card %s (%s) does not match board space (%d,%d) which is %s (expected card ID: %s)",
					playedCard.ToEmojiString(), playedCard.ID, action.BoardPos.X, action.BoardPos.Y, targetSpace.DisplayValue, targetSpace.Card.ID)
			}
		}
		log.Printf("Player %s plays %s to place chip at (%d,%d)", player.Name, playedCard.ToEmojiString(), action.BoardPos.X, action.BoardPos.Y)
		targetSpace.OccupiedBy = playerID
	}

	player.removeCardFromHand(playedCard.ID)
	if _, err := g.drawCard(playerID); err != nil {
		log.Printf("Player %s could not draw card: %v", playerID, err)
	}

	newSequencesFormed := g.checkForSequencesAfterPlay(playerID, action.BoardPos.X, action.BoardPos.Y)
	if newSequencesFormed > 0 {
		player.Sequences += newSequencesFormed
		log.Printf("Player %s formed %d new sequence(s)! Total sequences: %d", player.Name, newSequencesFormed, player.Sequences)
		if player.Sequences >= g.NumSequencesToWin {
			g.GamePhase = "Finished"
			g.Winner = playerID
			log.Printf("Game Over! Player %s wins!", player.Name)
		}
	}

	if g.GamePhase == "InProgress" {
		g.CurrentTurnIndex = (g.CurrentTurnIndex + 1) % len(g.PlayerOrder)
	}
	return nil
}

// HandleDeadCard allows a player to discard a dead card and draw a new one.
func (g *Game) HandleDeadCard(playerID string, cardID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.GamePhase != "InProgress" {
		return fmt.Errorf("game is not in progress")
	}
	if len(g.PlayerOrder) == 0 || g.CurrentTurnIndex >= len(g.PlayerOrder) || g.PlayerOrder[g.CurrentTurnIndex] != playerID {
		return fmt.Errorf("it's not player %s's turn", playerID)
	}

	player, ok := g.Players[playerID]
	if !ok {
		return fmt.Errorf("player %s not found", playerID)
	}

	deadCardInHand, hasCard := player.GetCardFromHand(cardID)
	if !hasCard {
		return fmt.Errorf("player %s does not have card %s", playerID, cardID)
	}
	if deadCardInHand.Rank == Jack {
		return fmt.Errorf("jacks cannot be dead cards")
	}

	isActuallyDead := true
	spotsForThisCard := 0
	for r := 0; r < BoardSize; r++ {
		for c := 0; c < BoardSize; c++ {
			space := g.Board[r][c]
			if space.Card != nil && space.Card.ID == deadCardInHand.ID {
				spotsForThisCard++
				if space.OccupiedBy == "" || space.OccupiedBy == "CORNER" {
					isActuallyDead = false
					break
				}
			}
		}
		if !isActuallyDead {
			break
		}
	}

	if spotsForThisCard == 0 {
		log.Printf("Error: Card %s (%s) declared dead by %s, but no spots found on board for this card ID. Check board layout.",
			deadCardInHand.ToEmojiString(), deadCardInHand.ID, player.Name)
		return fmt.Errorf("card %s not found on board layout, cannot be dead", deadCardInHand.ToEmojiString())
	}
	if !isActuallyDead {
		return fmt.Errorf("card %s (%s) is not dead, an available spot exists", deadCardInHand.ToEmojiString(), deadCardInHand.ID)
	}

	log.Printf("Player %s declares %s (%s) as a dead card.", player.Name, deadCardInHand.ToEmojiString(), deadCardInHand.ID)
	player.removeCardFromHand(deadCardInHand.ID)

	if _, err := g.drawCard(playerID); err != nil {
		log.Printf("Player %s could not draw replacement card: %v", playerID, err)
	}

	if g.GamePhase == "InProgress" {
		g.CurrentTurnIndex = (g.CurrentTurnIndex + 1) % len(g.PlayerOrder)
	}
	return nil
}

// checkForSequencesAfterPlay
func (g *Game) checkForSequencesAfterPlay(playerID string, x, y int) int {
	sequencesFound := 0
	dirs := [][2]int{{0, 1}, {1, 0}, {1, 1}, {1, -1}}

	for _, dir := range dirs {
		count := 1
		chipsInSequence := []Position{{X: x, Y: y}}

		for i := 1; i < 5; i++ {
			nx, ny := x+dir[0]*i, y+dir[1]*i
			if nx < 0 || nx >= BoardSize || ny < 0 || ny >= BoardSize {
				break
			}
			space := g.Board[nx][ny]
			if space.OccupiedBy == playerID || (space.IsCorner && !space.IsLocked) {
				count++
				chipsInSequence = append(chipsInSequence, Position{X: nx, Y: ny})
			} else {
				break
			}
		}
		for i := 1; i < 5; i++ {
			nx, ny := x-dir[0]*i, y-dir[1]*i
			if nx < 0 || nx >= BoardSize || ny < 0 || ny >= BoardSize {
				break
			}
			space := g.Board[nx][ny]
			if space.OccupiedBy == playerID || (space.IsCorner && !space.IsLocked) {
				count++
				chipsInSequence = append(chipsInSequence, Position{X: nx, Y: ny})
			} else {
				break
			}
		}

		if count >= 5 {
			isNewSequence := false
			for _, pos := range chipsInSequence {
				boardChip := g.Board[pos.X][pos.Y]
				if !boardChip.IsLocked || boardChip.IsCorner || (pos.X == x && pos.Y == y) {
					isNewSequence = true
					break
				}
			}
			if isNewSequence {
				sequencesFound++
				log.Printf("Sequence of %d found for player %s at (%d,%d) in dir (%d,%d)", count, playerID, x, y, dir[0], dir[1])
				for _, pos := range chipsInSequence {
					if !g.Board[pos.X][pos.Y].IsCorner {
						g.Board[pos.X][pos.Y].IsLocked = true
					}
				}
			}
		}
	}
	return sequencesFound
}

// --- WebSocket Handling ---

// ClientMessage
type ClientMessage struct {
	ActionType string       `json:"actionType"`
	Payload    PlayerAction `json:"payload"`
}

// PlayerAction
type PlayerAction struct {
	GameID         string   `json:"gameId,omitempty"`
	PlayerName     string   `json:"playerName,omitempty"`
	CardID         string   `json:"cardId,omitempty"`
	BoardPos       Position `json:"boardPos"`
	MaxPlayers     int      `json:"maxPlayers,omitempty"`
	SequencesToWin int      `json:"sequencesToWin,omitempty"`
}

// broadcastGameState
func (g *Game) broadcastGameState(messageType string, specificPayload interface{}) {
	g.mu.Lock()
	defer g.mu.Unlock()

	type BroadcastPlayer struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		ChipColor   string `json:"chipColor"`
		Sequences   int    `json:"sequences"`
		IsConnected bool   `json:"isConnected"`
		HandCount   int    `json:"handCount"`
		IsMyTurn    bool   `json:"isMyTurn"`
	}

	broadcastPlayers := make(map[string]BroadcastPlayer)
	for pid, p := range g.Players {
		isMyTurn := false
		if g.GamePhase == "InProgress" && len(g.PlayerOrder) > 0 && g.CurrentTurnIndex < len(g.PlayerOrder) {
			isMyTurn = (g.PlayerOrder[g.CurrentTurnIndex] == pid)
		}
		broadcastPlayers[pid] = BroadcastPlayer{
			ID: p.ID, Name: p.Name, ChipColor: p.ChipColor, Sequences: p.Sequences,
			IsConnected: p.IsConnected, HandCount: len(p.Hand), IsMyTurn: isMyTurn,
		}
	}

	currentTurnPlayerID := ""
	if g.GamePhase == "InProgress" && len(g.PlayerOrder) > 0 && g.CurrentTurnIndex < len(g.PlayerOrder) {
		currentTurnPlayerID = g.PlayerOrder[g.CurrentTurnIndex]
	}

	gameStateForBroadcast := struct {
		Type                string                           `json:"type"`
		GameID              string                           `json:"gameId"`
		Board               [BoardSize][BoardSize]BoardSpace `json:"board"`
		Players             map[string]BroadcastPlayer       `json:"players"`
		PlayerOrder         []string                         `json:"playerOrder"`
		CurrentTurnPlayerID string                           `json:"currentTurnPlayerId"`
		GamePhase           string                           `json:"gamePhase"`
		Winner              string                           `json:"winner,omitempty"`
		NumSequencesToWin   int                              `json:"numSequencesToWin"`
		MaxPlayers          int                              `json:"maxPlayers"`
		HostID              string                           `json:"hostId"`
		DrawPileCount       int                              `json:"drawPileCount"`
		Message             string                           `json:"message,omitempty"`
		Details             interface{}                      `json:"details,omitempty"`
	}{
		Type: messageType, GameID: g.ID, Board: g.Board, Players: broadcastPlayers, PlayerOrder: g.PlayerOrder,
		CurrentTurnPlayerID: currentTurnPlayerID, GamePhase: g.GamePhase, Winner: g.Winner,
		NumSequencesToWin: g.NumSequencesToWin, MaxPlayers: g.MaxPlayers, HostID: g.HostID,
		DrawPileCount: g.DrawPileCount, Details: specificPayload,
	}

	for playerIDLoop, player := range g.Players {
		if player.Conn != nil && player.IsConnected {
			if err := player.Conn.WriteJSON(gameStateForBroadcast); err != nil {
				log.Printf("Error broadcasting game state to player %s: %v", player.ID, err)
			}

			visibleHandIDs := make([]string, len(player.Hand))
			for i, cardInHand := range player.Hand {
				visibleHandIDs[i] = cardInHand.ID
			}
			handMsg := struct {
				Type string   `json:"type"`
				Hand []string `json:"hand"`
			}{Type: "HAND_UPDATE", Hand: visibleHandIDs}

			if player.Conn != nil && player.IsConnected {
				if err := player.Conn.WriteJSON(handMsg); err != nil {
					log.Printf("Error sending hand update to player %s (%s): %v", player.Name, playerIDLoop, err)
				}
			}
		}
	}
	log.Printf("Broadcasted game state for game %s, type: %s", g.ID, messageType)
}

// sendError
func sendError(conn *websocket.Conn, gameID string, errorMessage string) {
	errPayload := struct {
		Type   string `json:"type"`
		GameID string `json:"gameId,omitempty"`
		Error  string `json:"error"`
	}{"ERROR", gameID, errorMessage}
	if conn != nil {
		if err := conn.WriteJSON(errPayload); err != nil {
			log.Printf("Error sending error: %v", err)
		}
	}
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}
	defer conn.Close()

	playerID := generateID()
	var currentGame *Game
	var currentPlayer *Player
	log.Printf("Player %s connected via WebSocket.", playerID)

	for {
		var msg ClientMessage
		err := conn.ReadJSON(&msg)
		if err != nil {
			log.Printf("Read error from %s: %v", playerID, err)
			if currentPlayer != nil && currentGame != nil {
				currentGame.mu.Lock()
				if p, ok := currentGame.Players[currentPlayer.ID]; ok {
					p.IsConnected = false
					log.Printf("Player %s (%s) disconnected from game %s.", p.Name, playerID, currentGame.ID)

					allDisconnected := true
					for _, playerInGame := range currentGame.Players {
						if playerInGame.IsConnected {
							allDisconnected = false
							break
						}
					}
					if allDisconnected && currentGame.GamePhase != "Finished" {
						log.Printf("All players disconnected from game %s. Removing game.", currentGame.ID)
						gamesMu.Lock()
						delete(games, currentGame.ID)
						gamesMu.Unlock()
					} else {
						currentGame.broadcastGameState("GAME_UPDATE", map[string]string{"message": fmt.Sprintf("Player %s disconnected", p.Name)})
					}
				}
				currentGame.mu.Unlock()
			}
			break
		}

		log.Printf("Received action from %s: %s, Payload: %+v", playerID, msg.ActionType, msg.Payload)

		switch msg.ActionType {
		case "CREATE_GAME":
			gamesMu.Lock()
			game := NewGame(playerID, msg.Payload.PlayerName, msg.Payload.MaxPlayers, msg.Payload.SequencesToWin)
			games[game.ID] = game
			gamesMu.Unlock()
			currentGame = game

			player, errAdd := currentGame.AddPlayer(playerID, msg.Payload.PlayerName, conn)
			if errAdd != nil {
				sendError(conn, game.ID, fmt.Sprintf("Failed to add host to game: %v", errAdd))
				gamesMu.Lock()
				delete(games, game.ID)
				gamesMu.Unlock()
				return
			}
			currentPlayer = player
			log.Printf("Player %s (%s) created game %s as host.", currentPlayer.Name, playerID, currentGame.ID)
			currentGame.broadcastGameState("GAME_CREATED", nil)

		case "JOIN_GAME":
			gamesMu.Lock()
			game, exists := games[msg.Payload.GameID]
			gamesMu.Unlock()
			if !exists {
				sendError(conn, msg.Payload.GameID, "Game not found.")
				continue
			}

			currentGame = game
			player, errAdd := currentGame.AddPlayer(playerID, msg.Payload.PlayerName, conn)
			if errAdd != nil {
				sendError(conn, currentGame.ID, fmt.Sprintf("Failed to join game: %v", errAdd))
				currentGame = nil
				continue
			}
			currentPlayer = player
			log.Printf("Player %s (%s) joined game %s.", currentPlayer.Name, playerID, currentGame.ID)
			currentGame.broadcastGameState("PLAYER_JOINED", map[string]string{"playerName": currentPlayer.Name, "playerId": currentPlayer.ID})

		case "START_GAME":
			if currentGame == nil {
				sendError(conn, "", "Not in a game.")
				continue
			}
			if currentPlayer == nil || currentGame.HostID != currentPlayer.ID {
				sendError(conn, currentGame.ID, "Only the host can start the game.")
				continue
			}
			if errS := currentGame.StartGame(currentPlayer.ID); errS != nil {
				sendError(conn, currentGame.ID, fmt.Sprintf("Failed to start game: %v", errS))
				continue
			}
			log.Printf("Game %s started by host %s.", currentGame.ID, currentPlayer.Name)
			currentGame.broadcastGameState("GAME_STARTED", nil)

		case "PLAY_ACTION":
			if currentGame == nil || currentPlayer == nil {
				sendError(conn, "", "Not in active game.")
				continue
			}

			errPlay := currentGame.PlayAction(currentPlayer.ID, msg.Payload)
			if errPlay != nil {
				sendError(conn, currentGame.ID, fmt.Sprintf("Invalid action: %v", errPlay))
				currentGame.broadcastGameState("GAME_UPDATE", map[string]string{"error": errPlay.Error()})
				continue
			}

			var playedCardDisplay string
			parsedPlayedCard, parseErr := parseCardID(msg.Payload.CardID)
			if parseErr == nil {
				playedCardDisplay = parsedPlayedCard.ToEmojiString()
			} else {
				playedCardDisplay = msg.Payload.CardID
			}

			detail := map[string]interface{}{
				"action": "PLAY_ACTION", "player": currentPlayer.Name,
				"cardPlayedDisplay": playedCardDisplay,
				"cardPlayedID":      msg.Payload.CardID,
				"pos":               msg.Payload.BoardPos,
			}
			if parsedPlayedCard != nil && parsedPlayedCard.Rank == Jack && (parsedPlayedCard.Suit == Clubs || parsedPlayedCard.Suit == Spades) {
				detail["removedChipAt"] = msg.Payload.BoardPos
			}
			currentGame.broadcastGameState("GAME_UPDATE", detail)
			if currentGame.GamePhase == "Finished" {
				log.Printf("Game %s finished. Winner: %s", currentGame.ID, currentGame.Winner)
			}

		case "DEAD_CARD":
			if currentGame == nil || currentPlayer == nil {
				sendError(conn, "", "Not in active game.")
				continue
			}

			errDead := currentGame.HandleDeadCard(currentPlayer.ID, msg.Payload.CardID)
			if errDead != nil {
				sendError(conn, currentGame.ID, fmt.Sprintf("Invalid dead card: %v", errDead))
				currentGame.broadcastGameState("GAME_UPDATE", map[string]string{"error": errDead.Error()})
				continue
			}
			detail := map[string]interface{}{
				"action": "DEAD_CARD", "player": currentPlayer.Name,
				"cardDeclaredDeadID": msg.Payload.CardID,
			}
			currentGame.broadcastGameState("GAME_UPDATE", detail)

		default:
			log.Printf("Unknown action: %s from %s", msg.ActionType, playerID)
			sendError(conn, "", fmt.Sprintf("Unknown action: %s", msg.ActionType))
		}
	}
}

// serveClient
func serveClient(w http.ResponseWriter, r *http.Request) {
	htmlFilePath := filepath.Join(StaticDir, ClientHTMLFile)
	if _, err := os.Stat(htmlFilePath); os.IsNotExist(err) {
		log.Printf("HTML client file not found at %s", htmlFilePath)
		http.Error(w, "HTML client not found", http.StatusNotFound)
		return
	}
	log.Printf("Serving client HTML from: %s", htmlFilePath)
	http.ServeFile(w, r, htmlFilePath)
}

func main() {
	if _, err := os.Stat(StaticDir); os.IsNotExist(err) {
		if err := os.MkdirAll(StaticDir, 0o755); err != nil {
			log.Fatalf("Failed to create static dir %s: %v", StaticDir, err)
		}
		log.Printf("Created static dir: %s. Place '%s' there.", StaticDir, ClientHTMLFile)
	}
	http.HandleFunc("/ws", handleWebSocket)
	http.HandleFunc("/", serveClient)
	port := "8080"
	log.Printf("Server starting on :%s. WebSocket: /ws, Client: /", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}
