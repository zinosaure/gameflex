import { appPath } from "../libs/appBase.js";
import React from 'react';
// A render failure must leave a usable exit instead of an empty viewport.
export default class RouteErrorBoundary extends React.Component {
    state = { failed: false };
    static getDerivedStateFromError() { return { failed: true }; }
    render() {
        if (this.state.failed) return <div role="alert" style={{ padding: 32, color: '#fff', background: '#211d30', minHeight: '100vh' }}><h1>Affichage interrompu</h1><p>Rechargez la page pour réessayer.</p><a href={appPath("/")} style={{ color: '#cfb6ff' }}>← Revenir aux jeux</a></div>;
        return this.props.children;
    }
}
