package lan

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// Browser — une recherche en cours : la liste de ce qu'elle a trouvé, tenue à
// jour par ce qu'elle entend.
type Browser struct {
	transport transport
	onChange  func([]Found)
	mu        sync.Mutex
	found     map[string]*foundEntry
	done      chan struct{}
}

type foundEntry struct {
	found  Found
	expiry *time.Timer
}

// Browse — chercher les serveurs en mode découverte : une question, puis
// l'écoute, jusqu'à la fin de `ctx`. `onChange` reçoit la liste entière à
// chaque arrivée et à chaque départ, triée par nom.
func Browse(ctx context.Context, onChange func([]Found)) (*Browser, error) {
	socket, err := openMulticast()
	if err != nil {
		return nil, err
	}

	return browseOn(ctx, socket, onChange)
}

func browseOn(ctx context.Context, socket transport, onChange func([]Found)) (*Browser, error) {
	browser := &Browser{transport: socket, onChange: onChange, found: map[string]*foundEntry{}, done: make(chan struct{})}
	go browser.listen()
	go func() {
		<-ctx.Done()
		_ = socket.Close()
	}()

	return browser, browser.Ask()
}

// Ask — poser la question, une fois. C'est la seule question de la recherche
// après la première : « Chercher de nouveau » est un geste, pas une minuterie.
func (browser *Browser) Ask() error {
	packet, err := question()
	if err != nil {
		return err
	}

	return browser.transport.Send(packet)
}

// Done — fermé quand la recherche s'arrête.
func (browser *Browser) Done() <-chan struct{} { return browser.done }

// List — ce qui est trouvé maintenant.
func (browser *Browser) List() []Found {
	browser.mu.Lock()
	defer browser.mu.Unlock()

	return browser.listLocked()
}

func (browser *Browser) listLocked() []Found {
	list := []Found{}
	for _, entry := range browser.found {
		list = append(list, entry.found)
	}
	sort.Slice(list, func(left, right int) bool {
		if list[left].Name != list[right].Name {
			return list[left].Name < list[right].Name
		}

		return list[left].Address < list[right].Address
	})

	return list
}

func (browser *Browser) listen() {
	defer close(browser.done)
	defer func() {
		browser.mu.Lock()
		defer browser.mu.Unlock()
		for _, entry := range browser.found {
			entry.expiry.Stop()
		}
	}()
	for {
		packet, from, err := browser.transport.Receive()
		if err != nil {
			return
		}
		browser.heard(packet, from.String())
	}
}

// announced — ce qu'une réponse dit d'une instance.
type announced struct {
	ttl     uint32
	port    uint16
	host    string
	txt     map[string]string
	withPTR bool
	// display — le nom de l'instance tel qu'annoncé (les clés sont en minuscules).
	display string
}

// heard — une réponse : chaque instance de notre service qu'elle nomme entre
// dans la liste, ou en sort (TTL 0).
func (browser *Browser) heard(packet []byte, from string) {
	var parser dnsmessage.Parser
	header, err := parser.Start(packet)
	if err != nil || !header.Response {
		return
	}
	if err := parser.SkipAllQuestions(); err != nil {
		return
	}
	instances := map[string]*announced{}
	of := func(name string) *announced {
		key := strings.ToLower(name)
		if instances[key] == nil {
			instances[key] = &announced{txt: map[string]string{}}
		}

		return instances[key]
	}
	service := strings.ToLower(ServiceName)
	for {
		resource, err := parser.Answer()
		if err != nil {
			if err := parser.SkipAllAuthorities(); err != nil {
				break
			}
			// Les enregistrements additionnels portent souvent le SRV et le TXT.
			for {
				extra, err := parser.Additional()
				if err != nil {
					break
				}
				collect(extra, service, of)
			}

			break
		}
		collect(resource, service, of)
	}

	browser.mu.Lock()
	changed := false
	for name, seen := range instances {
		if !seen.withPTR {
			continue
		}
		key := name + "\x00" + from
		if seen.ttl == 0 {
			if entry, known := browser.found[key]; known {
				entry.expiry.Stop()
				delete(browser.found, key)
				changed = true
			}

			continue
		}
		instance := strings.TrimSuffix(seen.display, "."+ServiceName)
		found := Found{Instance: instance, Name: seen.txt["name"], Link: seen.txt["link"], Address: from}
		if found.Name == "" {
			found.Name = instance
		}
		if found.Link == "" && seen.port != 0 {
			found.Link = "http://" + from + ":" + strconv.Itoa(int(seen.port))
		}
		lifetime := time.Duration(seen.ttl) * time.Second
		if entry, known := browser.found[key]; known {
			entry.expiry.Stop()
			changed = changed || entry.found != found
			entry.found = found
			entry.expiry = browser.expireAfter(key, lifetime)
		} else {
			browser.found[key] = &foundEntry{found: found, expiry: browser.expireAfter(key, lifetime)}
			changed = true
		}
	}
	list := browser.listLocked()
	browser.mu.Unlock()
	if changed && browser.onChange != nil {
		browser.onChange(list)
	}
}

// expireAfter — un serveur dont le TTL finit sans nouvelle annonce sort de la
// liste : un seul événement, borné.
func (browser *Browser) expireAfter(key string, lifetime time.Duration) *time.Timer {
	return time.AfterFunc(lifetime, func() {
		browser.mu.Lock()
		_, known := browser.found[key]
		delete(browser.found, key)
		list := browser.listLocked()
		browser.mu.Unlock()
		if known && browser.onChange != nil {
			browser.onChange(list)
		}
	})
}

// collect — un enregistrement de notre service, rangé avec son instance.
func collect(resource dnsmessage.Resource, service string, of func(string) *announced) {
	name := resource.Header.Name.String()
	switch body := resource.Body.(type) {
	case *dnsmessage.PTRResource:
		if strings.ToLower(name) == service {
			target := body.PTR.String()
			seen := of(target)
			seen.withPTR = true
			seen.ttl = resource.Header.TTL
			seen.display = target
		}
	case *dnsmessage.SRVResource:
		if strings.HasSuffix(strings.ToLower(name), "."+service) {
			seen := of(name)
			seen.port = body.Port
			seen.host = body.Target.String()
		}
	case *dnsmessage.TXTResource:
		if strings.HasSuffix(strings.ToLower(name), "."+service) {
			seen := of(name)
			for _, entry := range body.TXT {
				if key, value, ok := strings.Cut(entry, "="); ok {
					seen.txt[key] = value
				}
			}
		}
	}
}
