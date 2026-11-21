import { appPath } from "../libs/appBase.js";
import React from 'react';
// A render failure must leave a usable exit instead of an empty viewport.
export default class RouteErrorBoundary extends React.Component {
    state = { failed: false };
    static getDerivedStateFromError() { return { failed: true }; }
    componentDidUpdate(previous) {
        if(previous.resetKey!==this.props.resetKey&&this.state.failed)this.setState({failed:false});
    }
    render() {
        if (this.state.failed) return <div role="alert" style={{ padding: 32, color: '#fff', background: '#211d30', minHeight: '100vh' }}><h1>Affichage interrompu</h1><p>Le menu reste disponible. Vous pouvez réessayer cette rubrique.</p><button onClick={()=>this.setState({failed:false})}>Réessayer sur place</button><nav aria-label="Menu principal"><a href={appPath("/?section=library")}>Mes jeux</a> · <a href={appPath("/?section=management")}>Gestion / Archives</a> · <a href={appPath("/settings")}>Paramètres</a></nav><a href={appPath("/")} style={{ color: '#cfb6ff' }}>← Revenir aux jeux</a></div>;
        return this.props.children;
    }
}
