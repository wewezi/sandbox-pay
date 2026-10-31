import { unsafeCSS } from 'lit';
import stylesheet from '../static/scrollbars.css?inline';

// The input is a fixed local stylesheet, shared with the document's CSS import.
export const scrollbarStyles = unsafeCSS(stylesheet);
