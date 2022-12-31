import {
  // onMounted,
  ref,
} from 'vue';

import {
  useResourceStore,
} from '@/store/resource';

import { Message } from 'bkui-vue';

// import { useI18n } from 'vue-i18n';

export default (type: string, data: any, id?: number) => {
  // const { t } = useI18n();
  const loading = ref(false);
  const resourceStore = useResourceStore();

  // 新增
  const addData = () => {
    loading.value = true;
    resourceStore
      .add(type, data)
      .then(() => {
        Message({
          message: '添加成功',
          theme: 'success',
        });
      })
      .finally(() => {
        loading.value = false;
      });
  };

  // 更新
  const updateData = () => {
    loading.value = true;
    resourceStore
      .update(type, data, id)
      .then(() => {
        Message({
          message: '编辑成功',
          theme: 'success',
        });
      })
      .finally(() => {
        loading.value = false;
      });
  };

  // onMounted(addData);

  return {
    loading,
    addData,
    updateData,
  };
};

