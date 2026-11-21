(function(){
 if(!document.querySelector('.ps-console,.gf-phone-controller')||window.GameFlexTv?.sharedUI)return;
 // Component handlers run first. Standard dialogs use the scoped fallback below.
 // Do not let the outer window navigator consume the same direction a second time.
 document.addEventListener('keydown',event=>{if(event.gameflexTvNavigation)event.stopPropagation();});
 const visible=el=>el.getClientRects().length&&getComputedStyle(el).visibility!=='hidden';
 const control=()=>document.activeElement;
 const surface=()=>[...document.querySelectorAll('[aria-modal="true"],.offcanvas.show,.modal.show')].filter(visible).at(-1)||document.querySelector('.ps-console,.gf-phone-controller')||document.body;
 const selector='button:not(:disabled),a[href],input:not(:disabled),textarea:not(:disabled),select:not(:disabled),summary';
 const controls=()=>[...surface().querySelectorAll(selector)].filter(el=>visible(el)&&el.getAttribute('aria-disabled')!=='true');
 const first=()=>surface().querySelector('.ps-console-tile[aria-pressed="true"]')||controls()[0];
 const focusedControl=()=>{let el=control();if(!surface().contains(el)||!el?.matches(selector)||!visible(el)){el=first();el?.focus();}return el;};
 const focus=el=>{el?.focus({preventScroll:true});el?.scrollIntoView({block:'nearest',inline:'nearest'});};
 const stepSelect=(el,step)=>{let index=el.selectedIndex+step;while(index>=0&&index<el.options.length&&el.options[index].disabled)index+=step;if(index>=0&&index<el.options.length){el.selectedIndex=index;el.dispatchEvent(new Event('change',{bubbles:true}));}};
 const fallback=(el,key)=>{if(el.tagName==='SELECT'&&(key==='ArrowLeft'||key==='ArrowRight')){stepSelect(el,key==='ArrowRight'?1:-1);return;}const a=el.getBoundingClientRect(),ax=a.x+a.width/2,ay=a.y+a.height/2,dx=key==='ArrowLeft'?-1:key==='ArrowRight'?1:0,dy=key==='ArrowUp'?-1:key==='ArrowDown'?1:0;const next=controls().filter(x=>x!==el).map(x=>{const b=x.getBoundingClientRect(),px=b.x+b.width/2-ax,py=b.y+b.height/2-ay;return {el:x,ahead:px*dx+py*dy,score:Math.hypot(px,py)+Math.abs(px*dy-py*dx)*2};}).filter(x=>x.ahead>5).sort((a,b)=>a.score-b.score)[0];focus(next?.el);};
 window.GameFlexTv={sharedUI:true,
  returnToLibrary(){if(history.length>1)history.back();else location.replace('/?tv=1');},
  move(direction){const el=focusedControl();if(!el)return;const key={left:'ArrowLeft',right:'ArrowRight',up:'ArrowUp',down:'ArrowDown'}[direction];if(!key)return;const event=new KeyboardEvent('keydown',{key,code:key,bubbles:true,cancelable:true});event.gameflexTvNavigation=true;el.dispatchEvent(event);if(!event.defaultPrevented)fallback(el,key);},
  ok(){const el=focusedControl();if(el?.matches('input:not([type=checkbox]):not([type=radio]),textarea')){el.focus();return 'input';}if(el?.tagName==='SELECT'){stepSelect(el,1);return 'button';}if(el?.matches('.gf-pad-key')){el.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true,cancelable:true}));el.dispatchEvent(new KeyboardEvent('keyup',{key:'Enter',bubbles:true,cancelable:true}));}else el?.click();return 'button';},
  menu(){if(surface().matches('[aria-modal="true"],.offcanvas.show,.modal.show')){this.back();return;}const button=document.querySelector('.ps-avatar,.ps-footer>button');if(button)button.click();else focus(first());},
  back(){const dialog=surface();if(dialog.matches('[aria-modal="true"],.offcanvas.show,.modal.show')){const close=dialog.querySelector('button[aria-label^="Fermer"],button[aria-label="Retour"],button.btn-close');if(close)close.click();else (focusedControl()||dialog).dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',code:'Escape',bubbles:true,cancelable:true}));return;}if(location.pathname==='/'&&!location.search.includes('section='))location.href='gameflex-tv://settings';else if(location.pathname!=='/')location.href='/?tv=1';else history.back();}
 };
 if(control()===document.body)first()?.focus();
})();
