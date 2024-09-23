import ReactDOM from 'react-dom/client';
import './index.css';
import SharingFileScreen from './App';
import { Toaster } from './components/ui/toaster';

console.log(process.env.REACT_APP_SERVER_URL);
console.log(process.env.REACT_APP_WS_URL);

const root = ReactDOM.createRoot(
  document.getElementById('root') as HTMLElement
);
root.render(
    <main className='flex items-center justify-center w-screen h-screen'>
        <SharingFileScreen />
        <Toaster />
    </main>
)