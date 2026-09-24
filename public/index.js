/* 落地页：有 token 进订单，没有就去登录。整页只有这一件事。 */
import { getToken } from '/app.js';

location.replace(getToken() ? '/orders.html' : '/login.html');
