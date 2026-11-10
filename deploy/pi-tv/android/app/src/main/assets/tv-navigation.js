(function(){
 if(!document.querySelector('.ps-console')||window.GameFlexTv?.sharedUI)return;
 // React's control surface handles this event before the document. Do not let
 // the outer application's separate window navigator move the focus again.
 document.addEventListener('keydown',event=>{if(event.gameflexTvNavigation)event.stopPropagation();});
 const control=()=>document.activeElement;
 const surface=()=>document.querySelector('[aria-modal="true"]')||document.querySelector('.ps-console')||document.body;
 const first=()=>surface().querySelector('.ps-console-tile[aria-pressed="true"]')||surface().querySelector('button:not(:disabled),a[href],input,select');
 const focusedControl=()=>{let el=control();if(!surface().contains(el)||!el?.matches('button:not(:disabled),a[href],input,textarea,select,summary')){el=first();el?.focus();}return el;};
 window.GameFlexTv={sharedUI:true,
  returnToLibrary(){if(history.length>1)history.back();else location.replace('/?tv=1');},
  move(direction){const el=focusedControl();if(!el)return;const key={left:'ArrowLeft',right:'ArrowRight',up:'ArrowUp',down:'ArrowDown'}[direction];if(key){const event=new KeyboardEvent('keydown',{key,code:key,bubbles:true,cancelable:true});event.gameflexTvNavigation=true;el.dispatchEvent(event);}},
  ok(){const el=focusedControl();if(el?.matches('input,textarea,select')){el.focus();return 'input';}el?.click();return 'button';},
  back(){const dialog=document.querySelector('[aria-modal="true"]');if(dialog){const close=dialog.querySelector('button[aria-label^="Fermer"],button[aria-label="Retour"]');if(close)close.click();else (control()||dialog).dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true,cancelable:true}));return;}if(location.pathname==='/'&&!location.search.includes('section='))location.href='gameflex-tv://settings';else if(location.pathname!=='/')location.href='/?tv=1';else history.back();}
 };
 if(control()===document.body)first()?.focus();
})();
