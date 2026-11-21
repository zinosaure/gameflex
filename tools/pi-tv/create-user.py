#!/usr/bin/env python3
"""Create a standard GameFlex account using the official administrator API.
Passwords are entered privately on the terminal, never stored or printed.
"""
import argparse
import getpass
import json
import sys
import urllib.parse
import urllib.request
import urllib.error

class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None

def request(base, path, data, token=None):
    headers = {'Content-Type': 'application/json'}
    if token:
        headers['Authorization'] = 'Bearer ' + token
    req = urllib.request.Request(base + path, data=json.dumps(data).encode(), headers=headers, method='POST')
    with urllib.request.build_opener(NoRedirect).open(req, timeout=15) as r:
        payload = json.loads(r.read(1024 * 1024))
        if payload.get('state') == 'failed': raise ValueError('refused')
        return r.status, payload

def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--server', default='http://127.0.0.1:2000')
    p.add_argument('--username', default='unknow')
    p.add_argument('--admin', default='root')
    a = p.parse_args()
    u = urllib.parse.urlsplit(a.server)
    if u.scheme not in ('http', 'https') or u.hostname not in ('127.0.0.1', 'localhost') or u.username or u.password or u.query or u.fragment or u.path not in ('','/'):
        p.error('Exécuter sur le Pi avec une URL de boucle locale ; ne pas envoyer les mots de passe sur le LAN.')
    if not sys.stdin.isatty() or not sys.stderr.isatty():
        p.error('Un terminal privé est requis pour la saisie masquée ; aucun mot de passe en argument.')
    try:
        admin_password = getpass.getpass('Mot de passe administrateur existant (masqué) : ')
        password = getpass.getpass('Mot de passe du nouveau compte (masqué) : ')
        confirm = getpass.getpass('Confirmer le nouveau mot de passe (masqué) : ')
        if password != confirm or len(password) < 8:
            p.exit(1, 'Confirmation différente ou mot de passe trop court. Aucun compte créé.\n')
        _, auth = request(a.server.rstrip('/'), '/api/v1/auth/login', {'username':a.admin, 'password':admin_password})
        token = auth['item']['access_token']
        status, result = request(a.server.rstrip('/'), '/api/v1/users', {'username':a.username, 'display_name':a.username, 'password':password, 'role_level':2}, token)
        item = result.get('item', {})
        if status != 201 or item.get('username') != a.username or item.get('role_level') != 2:
            p.exit(1, 'Réponse inattendue : vérifier le compte dans les paramètres, sans répéter automatiquement la création.\n')
        print('Compte standard créé ; mot de passe non enregistré dans les fichiers du projet.')
    except urllib.error.HTTPError as e:
        # Never echo response bodies, requests, passwords or bearer tokens.
        p.exit(1, f'API : HTTP {e.code}. Aucun compte existant modifié ; vérifier l’état dans GameFlex.\n')
    except (OSError, ValueError, KeyError):
        p.exit(1, 'Création non confirmée : connexion ou réponse API indisponible. Aucun secret affiché.\n')
if __name__ == '__main__': main()
