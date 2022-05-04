# 学习使用

因为实验需要去爬取网络上的软件，所以需要学习一下爬虫的编写方法


## onlyfreewares.com分析

我们只需要爬取所有详情页上的下载链接就好了，并不需要下载所有的exe文件

只有详情页包含我们需要的exe下载链接，且下载链接的对应元素`id=first_link`

首页->列表页：通过Category展位实现

首页->详情页：通过推荐, billiboard, 以及Special Freewares栏目实现

详情页->列表页：通过推荐列表实现


## 系统设计

Crawler: crawl for .exe links

Downloader: download .exe links

## go query selector cheatsheet

这个东西基本上就是css选择器

```
# 按id选择
#div1: 按id选择

# 按class选择
.name: 按class选择

# 按属性筛选
div[class]: 按属性选择
div[class=name]: 
div[lang!=zh]:

# 按相对位置筛选 
body > div: parent > child选择，相对父元素
div[lang=zh]+p: prev+next选择，相对上一个兄弟
div[lang=zh]~p: prev~next选择，并不一定要是相邻兄弟

# 按内容筛选
div:contains(DIV2): 选择包含指定文本内容的元素
div:has(div)：选择包含指定子元素的元素 

# 按绝对位置筛选
div:first-child: :first-child选择，选择父元素的第一个子元素
div:nth-child(3): 选择第三个子元素
div:first-of-type: 选择第一个符合类型的元素
div:last-of-child: 略
div:last-of-type: 略
div:only-child: 在其父元素下，只有自己一个子元素
div:only-of-type: 在其父元素下，只有自己一个符合类型

# 逻辑组合
div,span: 逻辑或

# mutiple attribute selector
input[id][name$='man']: 指定多个属性都要满足
.class1.class2: 要同时满足两个class
```






## 参考

- [golang goquery selector(选择器) 示例大全](https://www.flysnow.org/2018/01/20/golang-goquery-examples-selector.html#%E5%9F%BA%E4%BA%8Ehtml-element-%E5%85%83%E7%B4%A0%E7%9A%84%E9%80%89%E6%8B%A9%E5%99%A8)
