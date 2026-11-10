"""Fixed provider registry; user requests contain IDs, never arbitrary URLs."""
from freeroms import ProviderError

def provider(identifier):
    if isinstance(identifier,str):
        if identifier.startswith('freeroms:'):
            import freeroms
            return freeroms
        if identifier.startswith('romspure:'):
            import romspure
            return romspure
    raise ProviderError('Identifiant du catalogue invalide.')

def decode(identifier):
    parts=provider(identifier).decode(identifier)
    return parts[0],parts[-1]

def payload(data,identifier,progress=None):
    from source_settings import enabled
    if not enabled(data,identifier.split(':',1)[0]):raise ProviderError('Fournisseur désactivé dans les paramètres.')
    return provider(identifier).payload(data,identifier,progress=progress)
