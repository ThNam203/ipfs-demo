import React from 'react';
import ReactDOM from 'react-dom/client';
import './index.css';
import SharingFileScreen from './App';
import { Toaster } from './components/ui/toaster';


const root = ReactDOM.createRoot(
  document.getElementById('root') as HTMLElement
);
root.render(
    <main className='flex items-center justify-center w-screen h-screen'>
        <SharingFileScreen />
        <Toaster />
    </main>
)