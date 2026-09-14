import { authStore } from './auth';
import { taskStore } from './task';
import { articleStore } from './article';

export const stores = {
  auth: authStore,
  task: taskStore,
  article: articleStore,
};

export { authStore, taskStore, articleStore };
