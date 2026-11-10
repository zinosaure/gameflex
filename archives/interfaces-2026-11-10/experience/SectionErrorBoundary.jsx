import React from 'react';

// This boundary belongs inside the shell: a failed section must not remove its
// navigation or require a full page reload (and lose the current filters).
export default class SectionErrorBoundary extends React.Component {
    state = { failed: false, attempt: 0 };
    static getDerivedStateFromError() { return { failed: true }; }
    componentDidUpdate(previous) {
        if (previous.resetKey !== this.props.resetKey && this.state.failed) this.setState({ failed: false });
    }
    retry = () => {
        this.setState(state => ({ failed: false, attempt: state.attempt + 1 }));
        this.props.onRetry?.();
    };
    render() {
        if (this.state.failed) return <section className="gf-section-error" role="alert"><h2>Affichage interrompu</h2><p>Cette rubrique n’a pas pu être affichée. Le menu reste accessible.</p><button onClick={this.retry}>Réessayer sur place</button></section>;
        return <React.Fragment key={this.state.attempt}>{this.props.children}</React.Fragment>;
    }
}
