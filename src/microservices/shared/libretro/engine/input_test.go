package engine

import "testing"

// Le lot que la page envoie (le même au serveur et au téléphone), et ce que
// le cœur lit ensuite par input_state.
func TestInputsButtonsAndMask(t *testing.T) {
	var in inputs
	if err := in.apply(0, []byte(`[{"button":3,"value":1},{"button":8,"value":1}]`)); err != nil {
		t.Fatal(err)
	}

	if got := in.state(0, deviceJoypad, 0, 3); got != 1 {
		t.Fatalf("START (3) : %d, attendu 1", got)
	}
	if got := in.state(0, deviceJoypad, 0, joypadIDMask); got != 1<<3|1<<8 {
		t.Fatalf("masque : %b, attendu %b", got, 1<<3|1<<8)
	}
	if got := in.state(1, deviceJoypad, 0, 3); got != 0 {
		t.Fatalf("la manette 2 n'a rien reçu : %d", got)
	}

	if err := in.apply(0, []byte(`[{"button":3,"value":0}]`)); err != nil {
		t.Fatal(err)
	}
	if got := in.state(0, deviceJoypad, 0, 3); got != 0 {
		t.Fatalf("START relâché : %d", got)
	}
}

func TestInputsAnalogAndPointer(t *testing.T) {
	var in inputs
	if err := in.apply(1, []byte(`[{"axis":1,"x":-32767,"y":1200},{"pointer":1,"x":100,"y":-200}]`)); err != nil {
		t.Fatal(err)
	}

	if got := in.state(1, deviceAnalog, 1, 0); got != -32767 {
		t.Fatalf("stick droit, x : %d", got)
	}
	if got := in.state(1, deviceAnalog, 1, 1); got != 1200 {
		t.Fatalf("stick droit, y : %d", got)
	}
	if got := in.state(1, devicePointer, 0, pointerIDPress); got != 1 {
		t.Fatalf("doigt posé : %d", got)
	}
	if got := in.state(1, devicePointer, 1, pointerIDPress); got != 0 {
		t.Fatalf("un second doigt n'existe pas : %d", got)
	}

	// Le doigt se lève : la position reste (voir apply).
	if err := in.apply(1, []byte(`[{"pointer":0,"x":100,"y":-200}]`)); err != nil {
		t.Fatal(err)
	}
	if got := in.state(1, devicePointer, 0, pointerIDX); got != 100 {
		t.Fatalf("la position doit rester au relâchement : %d", got)
	}
	if got := in.state(1, devicePointer, 0, pointerIDPress); got != 0 {
		t.Fatalf("doigt levé : %d", got)
	}
}

func TestInputsRejects(t *testing.T) {
	var in inputs
	if err := in.apply(0, []byte(`pas du json`)); err == nil {
		t.Fatal("un lot illisible doit être refusé")
	}
	if err := in.apply(MaxPorts, []byte(`[]`)); err == nil {
		t.Fatal("un port hors limites doit être refusé")
	}
	if got := in.state(MaxPorts+3, deviceJoypad, 0, 0); got != 0 {
		t.Fatalf("un port hors limites se lit 0 : %d", got)
	}
}

// Le masque des seize boutons (RETRO_DEVICE_ID_JOYPAD_MASK) — repris du
// serveur, où l'absence de ce cas faisait lire « rien d'appuyé » à tout cœur
// qui interroge le masque (détection de combinaisons, raccourcis).
func TestInputsJoypadMaskCases(t *testing.T) {
	cases := []struct {
		name    string
		buttons map[int]int16
		want    int16
	}{
		{"rien d'appuyé", nil, 0},
		{"B (0)", map[int]int16{0: 1}, 1 << 0},
		{"SELECT+L1+R1, la bascule analogique", map[int]int16{2: 1, 10: 1, 11: 1}, 1<<2 | 1<<10 | 1<<11},
		{"R3 (15), le haut de la plage", map[int]int16{15: 1}, -32768},
		{"16 et au-delà ignorés", map[int]int16{16: 1, 31: 1}, 0},
		{"une valeur autre que 1 compte", map[int]int16{0: 32767}, 1 << 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var in inputs
			for button, value := range c.buttons {
				in.buttons[0][button] = value
			}
			if got := in.state(0, deviceJoypad, 0, joypadIDMask); got != c.want {
				t.Errorf("masque = %d, attendu %d", got, c.want)
			}
		})
	}
}

// Un joueur qui part relâche tout : une touche tenue ne reste pas enfoncée.
func TestInputsClear(t *testing.T) {
	var in inputs
	if err := in.apply(1, []byte(`[{"button":8,"value":1},{"axis":0,"x":100,"y":-100},{"pointer":1,"x":5,"y":5}]`)); err != nil {
		t.Fatal(err)
	}
	in.clear(1)
	if in.state(1, deviceJoypad, 0, 8) != 0 || in.state(1, deviceAnalog, 0, 0) != 0 || in.state(1, devicePointer, 0, pointerIDPress) != 0 {
		t.Fatal("le port libéré garde une entrée")
	}
}

// La bascule analogique de la PS1 : SELECT+L1+R1 seuls, puis plus rien.
func TestInputsForceCombo(t *testing.T) {
	var in inputs
	in.buttons[0][0] = 1 // un bouton tenu par ailleurs : la combinaison l'efface
	in.forceCombo(true)
	if got := in.state(0, deviceJoypad, 0, joypadIDMask); got != 1<<2|1<<10|1<<11 {
		t.Fatalf("combinaison : %b", got)
	}
	in.forceCombo(false)
	if got := in.state(0, deviceJoypad, 0, joypadIDMask); got != 0 {
		t.Fatalf("après la combinaison : %b", got)
	}
}
